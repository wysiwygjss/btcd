// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
)

// BitcoinCoreClient is a BalanceChecker backed by a local Bitcoin Core
// (bitcoind) JSON-RPC server. It uses scanblocks (when the node has
// -blockfilterindex=1) to detect whether an address ever appeared in a
// confirmed transaction, and scantxoutset to read any current UTXOs.
//
// Only the address string is ever sent to the RPC server; bip39scan never
// transmits mnemonics, seeds, extended keys, or private keys.
//
// Compared to an Esplora-compatible indexer, RPC lookups are significantly
// slower because each address may require scanning the node's compact block
// filters and/or UTXO set. Mempool (unconfirmed) balances are not reported.
type BitcoinCoreClient struct {
	// RPC is the low-level JSON-RPC caller. If nil, Balance constructs one
	// from the other fields on first use.
	RPC *bitcoindRPC

	// Host is the bitcoind RPC endpoint, e.g. "localhost:8332" or
	// "unix:///path/to/bitcoin/.cookie" (the unix socket path itself).
	Host string

	// User and Pass authenticate to bitcoind when CookiePath is empty.
	User string
	Pass string

	// CookiePath, if set, is read instead of User/Pass (bitcoind's default
	// cookie-file authentication).
	CookiePath string

	// DisableTLS should be true for the default HTTP RPC listener.
	DisableTLS bool

	// Timeout bounds each individual RPC round trip.
	Timeout time.Duration

	// MaxRetries is how many additional attempts are made after a
	// retryable failure before Balance gives up.
	MaxRetries int

	// RetryBaseDelay is the delay before the first retry; each subsequent
	// retry doubles it.
	RetryBaseDelay time.Duration

	// UseScanBlocks controls whether scanblocks is tried before falling
	// back to scantxoutset-only activity detection. When false, only
	// scantxoutset is used and fully-spent addresses are invisible.
	UseScanBlocks bool

	useScanBlocksMu sync.Mutex
	scanTxOutSetMu  sync.Mutex
}

type bitcoindRPC struct {
	url        string
	authHeader string
	client     *http.Client
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	ID     int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type scanTxOutSetResult struct {
	Success     bool               `json:"success"`
	TotalAmount float64            `json:"total_amount"`
	Unspents    []scanTxOutUnspent `json:"unspents"`
}

type scanTxOutUnspent struct {
	TxID   string  `json:"txid"`
	Amount float64 `json:"amount"`
}

type scanBlocksResult struct {
	RelevantBlocks []string `json:"relevant_blocks"`
	Completed      bool     `json:"completed"`
}

// NewBitcoinCoreClient returns a BitcoinCoreClient wired to host with the
// given credentials.
func NewBitcoinCoreClient(cfg BitcoinCoreRPCConfig) (*BitcoinCoreClient, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("rpc host is required")
	}

	rpc, err := newBitcoindRPC(cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	useScanBlocks := true
	if cfg.UseScanBlocks != nil {
		useScanBlocks = *cfg.UseScanBlocks
	}

	return &BitcoinCoreClient{
		RPC:            rpc,
		Host:           cfg.Host,
		User:           cfg.User,
		Pass:           cfg.Pass,
		CookiePath:     cfg.CookiePath,
		DisableTLS:     cfg.DisableTLS,
		Timeout:        timeout,
		MaxRetries:     cfg.MaxRetries,
		RetryBaseDelay: cfg.RetryBaseDelay,
		UseScanBlocks:  useScanBlocks,
	}, nil
}

// BitcoinCoreRPCConfig configures a BitcoinCoreClient.
type BitcoinCoreRPCConfig struct {
	Host           string
	User           string
	Pass           string
	CookiePath     string
	DisableTLS     bool
	Timeout        time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
	UseScanBlocks  *bool
}

func newBitcoindRPC(cfg BitcoinCoreRPCConfig) (*bitcoindRPC, error) {
	user, pass, err := rpcCredentials(cfg.User, cfg.Pass, cfg.CookiePath)
	if err != nil {
		return nil, err
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.Host,
		User:         user,
		Pass:         pass,
		DisableTLS:   cfg.DisableTLS,
		HTTPPostMode: true,
	}

	url, err := connCfgHTTPURL(connCfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	parsedAddr, err := rpcclient.ParseAddressString(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid rpc host %q: %w", cfg.Host, err)
	}

	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial(parsedAddr.Network(), parsedAddr.String())
		},
	}

	return &bitcoindRPC{
		url:        url,
		authHeader: basicAuthHeader(user, pass),
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}, nil
}

// connCfgHTTPURL mirrors rpcclient.ConnConfig.httpURL without importing
// unexported helpers.
func connCfgHTTPURL(cfg *rpcclient.ConnConfig) (string, error) {
	protocol := "http"
	if !cfg.DisableTLS {
		protocol = "https"
	}

	parsedAddr, err := rpcclient.ParseAddressString(cfg.Host)
	if err != nil {
		return "", fmt.Errorf("error parsing host %q: %w", cfg.Host, err)
	}

	switch parsedAddr.Network() {
	case "unix", "unixpacket":
		return protocol + "://unix", nil
	default:
		return protocol + "://" + cfg.Host, nil
	}
}

func rpcCredentials(user, pass, cookiePath string) (string, string, error) {
	if cookiePath != "" {
		return readRPCCookieFile(cookiePath)
	}
	if user == "" && pass == "" {
		return "", "", fmt.Errorf("one of -rpc-user/-rpc-pass or " +
			"-rpc-cookie-file is required")
	}
	return user, pass, nil
}

func readRPCCookieFile(path string) (string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to read rpc cookie file: %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(string(raw)), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed rpc cookie file %s", path)
	}
	return parts[0], parts[1], nil
}

func basicAuthHeader(user, pass string) string {
	auth := user + ":" + pass
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

func (r *bitcoindRPC) call(
	ctx context.Context,
	method string,
	params []any,
) (json.RawMessage, error) {

	body, err := json.Marshal(rpcRequest{
		JSONRPC: "1.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", r.authHeader)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("rpc authentication failed")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode,
			strings.TrimSpace(string(respBody)))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return rpcResp.Result, nil
}

func (c *BitcoinCoreClient) rpcClient() (*bitcoindRPC, error) {
	if c.RPC != nil {
		return c.RPC, nil
	}
	rpc, err := newBitcoindRPC(BitcoinCoreRPCConfig{
		Host:       c.Host,
		User:       c.User,
		Pass:       c.Pass,
		CookiePath: c.CookiePath,
		DisableTLS: c.DisableTLS,
		Timeout:    c.Timeout,
	})
	if err != nil {
		return nil, err
	}
	c.RPC = rpc
	return rpc, nil
}

// Balance implements BalanceChecker.
func (c *BitcoinCoreClient) Balance(
	ctx context.Context,
	address string,
) (AddressBalance, error) {

	desc := fmt.Sprintf("addr(%s)", address)

	var lastErr error
	delay := c.RetryBaseDelay
	if delay <= 0 {
		delay = time.Second
	}
	maxRetries := c.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return AddressBalance{}, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}

		balance, retryable, err := c.balanceOnce(ctx, desc)
		if err == nil {
			return balance, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			break
		}
	}

	return AddressBalance{}, fmt.Errorf("failed to fetch balance for %s "+
		"from bitcoind rpc at %s: %w", address, c.Host, lastErr)
}

func (c *BitcoinCoreClient) balanceOnce(
	ctx context.Context,
	desc string,
) (AddressBalance, bool, error) {

	rpc, err := c.rpcClient()
	if err != nil {
		return AddressBalance{}, false, err
	}

	hasHistory := false
	if c.shouldUseScanBlocks() {
		blocks, scanErr := c.scanBlocks(ctx, rpc, desc)
		if scanErr == nil {
			hasHistory = len(blocks) > 0
			if !hasHistory {
				return AddressBalance{}, false, nil
			}
		} else if isScanBlocksUnavailable(scanErr) {
			c.disableScanBlocks()
		} else if isRetryableRPCError(scanErr) {
			return AddressBalance{}, true, scanErr
		} else {
			return AddressBalance{}, false, scanErr
		}
	}

	utxoBalance, err := c.scanTxOutSet(ctx, rpc, desc)
	if err != nil {
		if isRetryableRPCError(err) {
			return AddressBalance{}, true, err
		}
		return AddressBalance{}, false, err
	}

	balance := AddressBalance{
		ConfirmedSats: utxoBalance.ConfirmedSats,
		TxCount:       utxoBalance.TxCount,
	}
	if hasHistory && balance.TxCount == 0 && balance.ConfirmedSats == 0 {
		balance.TxCount = 1
	}
	return balance, false, nil
}

func (c *BitcoinCoreClient) shouldUseScanBlocks() bool {
	c.useScanBlocksMu.Lock()
	defer c.useScanBlocksMu.Unlock()
	return c.UseScanBlocks
}

func (c *BitcoinCoreClient) disableScanBlocks() {
	c.useScanBlocksMu.Lock()
	defer c.useScanBlocksMu.Unlock()
	c.UseScanBlocks = false
}

func (c *BitcoinCoreClient) scanBlocks(
	ctx context.Context,
	rpc *bitcoindRPC,
	desc string,
) ([]string, error) {

	result, err := rpc.call(ctx, "scanblocks", []any{
		"start",
		[]string{desc},
		0,
	})
	if err != nil {
		return nil, err
	}

	var scanResult scanBlocksResult
	if err := json.Unmarshal(result, &scanResult); err != nil {
		return nil, fmt.Errorf("failed to decode scanblocks response: %w",
			err)
	}
	return scanResult.RelevantBlocks, nil
}

type utxoBalanceResult struct {
	ConfirmedSats int64
	TxCount       int
}

func (c *BitcoinCoreClient) scanTxOutSet(
	ctx context.Context,
	rpc *bitcoindRPC,
	desc string,
) (utxoBalanceResult, error) {

	c.scanTxOutSetMu.Lock()
	defer c.scanTxOutSetMu.Unlock()

	result, err := rpc.call(ctx, "scantxoutset", []any{
		"start",
		[]string{desc},
	})
	if err != nil {
		return utxoBalanceResult{}, err
	}

	var scanResult scanTxOutSetResult
	if err := json.Unmarshal(result, &scanResult); err != nil {
		return utxoBalanceResult{}, fmt.Errorf(
			"failed to decode scantxoutset response: %w", err)
	}
	if !scanResult.Success {
		return utxoBalanceResult{}, fmt.Errorf("scantxoutset did not " +
			"complete successfully")
	}

	confirmed := btcAmountToSats(scanResult.TotalAmount)
	return utxoBalanceResult{
		ConfirmedSats: confirmed,
		TxCount:       len(scanResult.Unspents),
	}, nil
}

func btcAmountToSats(btc float64) int64 {
	return int64(math.Round(btc * 1e8))
}

func isScanBlocksUnavailable(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "blockfilterindex") ||
		strings.Contains(msg, "block filter index") ||
		strings.Contains(msg, "method not found") ||
		strings.Contains(msg, "unknown method")
}

func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errorsIsNetwork(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "loading block index") ||
		strings.Contains(msg, "warmup") ||
		strings.Contains(msg, "work queue depth exceeded")
}

func errorsIsNetwork(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		(strings.Contains(msg, "EOF") && !strings.Contains(msg, "rpc error"))
}

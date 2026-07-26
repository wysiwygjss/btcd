// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AddressBalance summarizes the on-chain activity bip39scan found for a
// single derived address.
type AddressBalance struct {
	// ConfirmedSats is the confirmed balance, in satoshis (funded minus
	// spent outputs in already-mined blocks). It can be zero even for an
	// address with transaction history, if all of its funds have since
	// been spent.
	ConfirmedSats int64

	// UnconfirmedSats is the net effect of any still-unconfirmed
	// (mempool) transactions on the address's balance, in satoshis.
	UnconfirmedSats int64

	// TxCount is the total number of confirmed and unconfirmed
	// transactions touching the address.
	TxCount int
}

// HasActivity reports whether the address has ever been used: it either
// holds a nonzero balance right now, or has at least one transaction in its
// history (even if fully spent since).
func (b AddressBalance) HasActivity() bool {
	return b.TxCount > 0 || b.ConfirmedSats != 0 || b.UnconfirmedSats != 0
}

// BalanceChecker looks up the on-chain activity for a single address.
// Implementations must be safe for concurrent use.
type BalanceChecker interface {
	Balance(ctx context.Context, address string) (AddressBalance, error)
}

// EsploraClient is a BalanceChecker backed by an Esplora-compatible REST API
// (as served by blockstream.info, mempool.space, or a self-hosted esplora
// instance), using the "GET /address/:address" endpoint.
//
// Only the address string is ever sent to BaseURL; bip39scan never
// transmits mnemonics, seeds, extended keys, or private keys over the
// network. Point BaseURL at a self-hosted esplora instance (or use -offline
// entirely) to avoid revealing derived addresses to a third party.
type EsploraClient struct {
	// BaseURL is the esplora API root, e.g. "https://blockstream.info/api"
	// (no trailing slash required).
	BaseURL string

	// HTTPClient performs the actual requests. If nil, Balance
	// constructs a default *http.Client with Timeout applied.
	HTTPClient *http.Client

	// Timeout bounds each individual HTTP request when HTTPClient is
	// nil. Ignored if HTTPClient is set explicitly.
	Timeout time.Duration

	// MaxRetries is how many additional attempts are made after a
	// retryable failure (a network error, HTTP 429, or HTTP 5xx) before
	// Balance gives up and returns an error.
	MaxRetries int

	// RetryBaseDelay is the delay before the first retry; each
	// subsequent retry doubles it (simple exponential backoff).
	RetryBaseDelay time.Duration
}

// esploraAddressStats mirrors the subset of Esplora's
// "GET /address/:address" response bip39scan needs.
type esploraAddressStats struct {
	ChainStats   esploraStats `json:"chain_stats"`
	MempoolStats esploraStats `json:"mempool_stats"`
}

type esploraStats struct {
	FundedTxoSum int64 `json:"funded_txo_sum"`
	SpentTxoSum  int64 `json:"spent_txo_sum"`
	TxCount      int   `json:"tx_count"`
}

// httpClient returns the configured HTTP client, or a sensible
// timeout-bounded default if none was set.
func (c *EsploraClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// Balance implements BalanceChecker.
func (c *EsploraClient) Balance(ctx context.Context, address string) (AddressBalance, error) {
	url := strings.TrimRight(c.BaseURL, "/") + "/address/" + address

	var lastErr error
	delay := c.RetryBaseDelay
	if delay <= 0 {
		delay = time.Second
	}

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return AddressBalance{}, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}

		stats, retryable, err := c.fetchOnce(ctx, url)
		if err == nil {
			return AddressBalance{
				ConfirmedSats: stats.ChainStats.FundedTxoSum -
					stats.ChainStats.SpentTxoSum,
				UnconfirmedSats: stats.MempoolStats.FundedTxoSum -
					stats.MempoolStats.SpentTxoSum,
				TxCount: stats.ChainStats.TxCount +
					stats.MempoolStats.TxCount,
			}, nil
		}

		lastErr = err
		if !retryable {
			break
		}
	}

	return AddressBalance{}, fmt.Errorf("failed to fetch balance for %s "+
		"from %s: %w", address, c.BaseURL, lastErr)
}

// fetchOnce performs a single HTTP round trip. The returned bool reports
// whether the error (if any) is worth retrying.
func (c *EsploraClient) fetchOnce(
	ctx context.Context,
	url string,
) (esploraAddressStats, bool, error) {

	var stats esploraAddressStats

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return stats, false, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		// Network-level failures (timeouts, connection resets, DNS
		// hiccups) are transient by nature.
		return stats, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return stats, true, fmt.Errorf("http status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		// Esplora returns 404 with an empty body for well-formed but
		// invalid addresses. Since bip39scan only ever generates
		// addresses it derived itself, treat this the same as "never
		// used" rather than as an error.
		return stats, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return stats, false, fmt.Errorf("http status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return stats, false, fmt.Errorf("failed to decode response: %w",
			err)
	}
	return stats, false, nil
}

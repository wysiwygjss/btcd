// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBitcoinCoreClientBalanceUnused checks that an address with no
// scanblocks hits and no UTXOs reports no activity.
func TestBitcoinCoreClientBalanceUnused(t *testing.T) {
	const addr = "bc1qunusedaddress"

	srv := newMockBitcoindRPC(t, map[string]mockRPCHandler{
		"scanblocks": func(params []json.RawMessage) (any, error) {
			return scanBlocksResult{RelevantBlocks: nil, Completed: true}, nil
		},
	})
	defer srv.Close()

	client := &BitcoinCoreClient{
		RPC:           srv.client,
		UseScanBlocks: true,
	}

	balance, err := client.Balance(context.Background(), addr)
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if balance.HasActivity() {
		t.Fatalf("HasActivity() = true, want false")
	}
}

// TestBitcoinCoreClientBalanceWithUTXO checks that scantxoutset balances are
// parsed correctly when scanblocks reports relevant blocks.
func TestBitcoinCoreClientBalanceWithUTXO(t *testing.T) {
	const addr = "bc1qactiveaddress"

	srv := newMockBitcoindRPC(t, map[string]mockRPCHandler{
		"scanblocks": func(params []json.RawMessage) (any, error) {
			return scanBlocksResult{
				RelevantBlocks: []string{"abc123"},
				Completed:      true,
			}, nil
		},
		"scantxoutset": func(params []json.RawMessage) (any, error) {
			return scanTxOutSetResult{
				Success:     true,
				TotalAmount: 0.003,
				Unspents: []scanTxOutUnspent{
					{TxID: "tx1", Amount: 0.001},
					{TxID: "tx2", Amount: 0.002},
				},
			}, nil
		},
	})
	defer srv.Close()

	client := &BitcoinCoreClient{
		RPC:           srv.client,
		UseScanBlocks: true,
	}

	balance, err := client.Balance(context.Background(), addr)
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if balance.ConfirmedSats != 300000 {
		t.Errorf("ConfirmedSats = %d, want 300000", balance.ConfirmedSats)
	}
	if balance.TxCount != 2 {
		t.Errorf("TxCount = %d, want 2", balance.TxCount)
	}
	if !balance.HasActivity() {
		t.Fatalf("HasActivity() = false, want true")
	}
}

// TestBitcoinCoreClientBalanceSpentHistory checks that an address with
// confirmed history but no remaining UTXOs still counts as active.
func TestBitcoinCoreClientBalanceSpentHistory(t *testing.T) {
	const addr = "bc1qspentaddress"

	srv := newMockBitcoindRPC(t, map[string]mockRPCHandler{
		"scanblocks": func(params []json.RawMessage) (any, error) {
			return scanBlocksResult{
				RelevantBlocks: []string{"abc123"},
				Completed:      true,
			}, nil
		},
		"scantxoutset": func(params []json.RawMessage) (any, error) {
			return scanTxOutSetResult{
				Success:     true,
				TotalAmount: 0,
				Unspents:    nil,
			}, nil
		},
	})
	defer srv.Close()

	client := &BitcoinCoreClient{
		RPC:           srv.client,
		UseScanBlocks: true,
	}

	balance, err := client.Balance(context.Background(), addr)
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if !balance.HasActivity() {
		t.Fatalf("HasActivity() = false, want true for spent history")
	}
	if balance.TxCount != 1 {
		t.Errorf("TxCount = %d, want 1", balance.TxCount)
	}
}

// TestBitcoinCoreClientScanBlocksFallback checks that when scanblocks is
// unavailable, scantxoutset alone still reports current UTXOs.
func TestBitcoinCoreClientScanBlocksFallback(t *testing.T) {
	const addr = "bc1qfallbackaddress"

	var scanblocksCalled atomic.Bool

	srv := newMockBitcoindRPC(t, map[string]mockRPCHandler{
		"scanblocks": func(params []json.RawMessage) (any, error) {
			scanblocksCalled.Store(true)
			return nil, &rpcError{
				Code:    -1,
				Message: "Block filter index is not enabled",
			}
		},
		"scantxoutset": func(params []json.RawMessage) (any, error) {
			return scanTxOutSetResult{
				Success:     true,
				TotalAmount: 0.0001,
				Unspents: []scanTxOutUnspent{
					{TxID: "tx1", Amount: 0.0001},
				},
			}, nil
		},
	})
	defer srv.Close()

	client := &BitcoinCoreClient{
		RPC:           srv.client,
		UseScanBlocks: true,
	}

	balance, err := client.Balance(context.Background(), addr)
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if balance.ConfirmedSats != 10000 {
		t.Errorf("ConfirmedSats = %d, want 10000", balance.ConfirmedSats)
	}
	if !balance.HasActivity() {
		t.Fatalf("HasActivity() = false, want true")
	}
	if !scanblocksCalled.Load() {
		t.Fatalf("scanblocks was not called")
	}
	if client.shouldUseScanBlocks() {
		t.Fatalf("UseScanBlocks = true, want false after fallback")
	}
}

// TestBitcoinCoreClientSerializesScanTxOutSet checks that concurrent balance
// lookups serialize scantxoutset calls, because bitcoind only allows one at
// a time.
func TestBitcoinCoreClientSerializesScanTxOutSet(t *testing.T) {
	var (
		mu         sync.Mutex
		inScan     bool
		overlap    bool
		scanCalls  atomic.Int32
		blockCalls atomic.Int32
	)

	srv := newMockBitcoindRPC(t, map[string]mockRPCHandler{
		"scanblocks": func(params []json.RawMessage) (any, error) {
			blockCalls.Add(1)
			return scanBlocksResult{
				RelevantBlocks: []string{"abc123"},
				Completed:      true,
			}, nil
		},
		"scantxoutset": func(params []json.RawMessage) (any, error) {
			mu.Lock()
			scanCalls.Add(1)
			if inScan {
				overlap = true
			}
			inScan = true
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			inScan = false
			mu.Unlock()

			return scanTxOutSetResult{
				Success:     true,
				TotalAmount: 0.0001,
				Unspents: []scanTxOutUnspent{
					{TxID: "tx1", Amount: 0.0001},
				},
			}, nil
		},
	})
	defer srv.Close()

	client := &BitcoinCoreClient{
		RPC:           srv.client,
		UseScanBlocks: true,
	}

	const workers = 4
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := client.Balance(context.Background(),
				"bc1qconcurrentaddress")
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("Balance() returned unexpected error: %v", err)
		}
	}
	if overlap {
		t.Fatalf("scantxoutset calls overlapped")
	}
	if scanCalls.Load() != workers {
		t.Fatalf("scantxoutset called %d times, want %d", scanCalls.Load(),
			workers)
	}
	if blockCalls.Load() != workers {
		t.Fatalf("scanblocks called %d times, want %d", blockCalls.Load(),
			workers)
	}
}

type mockRPCHandler func(params []json.RawMessage) (any, error)

type mockBitcoindRPC struct {
	client *bitcoindRPC
	server *httptest.Server
}

func newMockBitcoindRPC(
	t *testing.T,
	handlers map[string]mockRPCHandler,
) *mockBitcoindRPC {

	t.Helper()

	mock := &mockBitcoindRPC{}
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		handler, ok := handlers[req.Method]
		if !ok {
			writeRPCError(w, &rpcError{
				Code:    -32601,
				Message: "method not found",
			})
			return
		}

		params := make([]json.RawMessage, 0, len(req.Params))
		for _, p := range req.Params {
			raw, err := json.Marshal(p)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			params = append(params, raw)
		}

		result, err := handler(params)
		if err != nil {
			if rpcErr, ok := err.(*rpcError); ok {
				writeRPCError(w, rpcErr)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeRPCResult(w, result)
	}))

	mock.client = &bitcoindRPC{
		url:        mock.server.URL,
		authHeader: basicAuthHeader("user", "pass"),
		client:     mock.server.Client(),
	}
	return mock
}

func (m *mockBitcoindRPC) Close() {
	m.server.Close()
}

func writeRPCResult(w http.ResponseWriter, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `{"result":%s,"error":null,"id":1}`, raw)
}

func writeRPCError(w http.ResponseWriter, rpcErr *rpcError) {
	fmt.Fprintf(w, `{"result":null,"error":{"code":%d,"message":%q},"id":1}`,
		rpcErr.Code, rpcErr.Message)
}

func TestBTCAmountToSats(t *testing.T) {
	tests := []struct {
		btc  float64
		want int64
	}{
		{0, 0},
		{1, 100_000_000},
		{0.00000001, 1},
		{0.003, 300_000},
	}
	for _, tc := range tests {
		if got := btcAmountToSats(tc.btc); got != tc.want {
			t.Errorf("btcAmountToSats(%v) = %d, want %d", tc.btc, got,
				tc.want)
		}
	}
}

// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestEsploraClientBalance checks that EsploraClient correctly parses a
// typical Esplora "GET /address/:address" response into an AddressBalance.
func TestEsploraClientBalance(t *testing.T) {
	const addr = "bc1qexampleaddress"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/address/"+addr {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{
			"address": %q,
			"chain_stats": {"funded_txo_sum": 500000, "spent_txo_sum": 200000, "tx_count": 3},
			"mempool_stats": {"funded_txo_sum": 10000, "spent_txo_sum": 0, "tx_count": 1}
		}`, addr)
	}))
	defer srv.Close()

	client := &EsploraClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	balance, err := client.Balance(context.Background(), addr)
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}

	if balance.ConfirmedSats != 300000 {
		t.Errorf("ConfirmedSats = %d, want 300000", balance.ConfirmedSats)
	}
	if balance.UnconfirmedSats != 10000 {
		t.Errorf("UnconfirmedSats = %d, want 10000", balance.UnconfirmedSats)
	}
	if balance.TxCount != 4 {
		t.Errorf("TxCount = %d, want 4", balance.TxCount)
	}
	if !balance.HasActivity() {
		t.Errorf("HasActivity() = false, want true")
	}
}

// TestEsploraClientBalanceUnused checks that an address with no activity at
// all reports HasActivity() == false.
func TestEsploraClientBalanceUnused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"chain_stats": {"funded_txo_sum": 0, "spent_txo_sum": 0, "tx_count": 0},
			"mempool_stats": {"funded_txo_sum": 0, "spent_txo_sum": 0, "tx_count": 0}
		}`)
	}))
	defer srv.Close()

	client := &EsploraClient{BaseURL: srv.URL, Timeout: 5 * time.Second}
	balance, err := client.Balance(context.Background(), "1anyaddress")
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if balance.HasActivity() {
		t.Errorf("HasActivity() = true, want false")
	}
}

// TestEsploraClientRetriesTransientErrors checks that Balance retries on a
// 503 response and succeeds once the server recovers, without exceeding
// MaxRetries.
func TestEsploraClientRetriesTransientErrors(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{
			"chain_stats": {"funded_txo_sum": 100, "spent_txo_sum": 0, "tx_count": 1},
			"mempool_stats": {"funded_txo_sum": 0, "spent_txo_sum": 0, "tx_count": 0}
		}`)
	}))
	defer srv.Close()

	client := &EsploraClient{
		BaseURL:        srv.URL,
		Timeout:        5 * time.Second,
		MaxRetries:     5,
		RetryBaseDelay: time.Millisecond,
	}
	balance, err := client.Balance(context.Background(), "1anyaddress")
	if err != nil {
		t.Fatalf("Balance() returned unexpected error: %v", err)
	}
	if balance.ConfirmedSats != 100 {
		t.Errorf("ConfirmedSats = %d, want 100", balance.ConfirmedSats)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server received %d attempts, want 3", got)
	}
}

// TestEsploraClientGivesUpAfterMaxRetries checks that Balance stops
// retrying (and returns an error) once MaxRetries is exhausted.
func TestEsploraClientGivesUpAfterMaxRetries(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := &EsploraClient{
		BaseURL:        srv.URL,
		Timeout:        5 * time.Second,
		MaxRetries:     2,
		RetryBaseDelay: time.Millisecond,
	}
	_, err := client.Balance(context.Background(), "1anyaddress")
	if err == nil {
		t.Fatalf("Balance() unexpectedly succeeded")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("server received %d attempts, want 3 (1 + MaxRetries)", got)
	}
}

// TestEsploraClientNonRetryableError checks that a 400 response is not
// retried.
func TestEsploraClientNonRetryableError(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "invalid address")
	}))
	defer srv.Close()

	client := &EsploraClient{
		BaseURL:        srv.URL,
		Timeout:        5 * time.Second,
		MaxRetries:     5,
		RetryBaseDelay: time.Millisecond,
	}
	_, err := client.Balance(context.Background(), "not-an-address")
	if err == nil {
		t.Fatalf("Balance() unexpectedly succeeded")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("server received %d attempts, want 1 (no retries)", got)
	}
}

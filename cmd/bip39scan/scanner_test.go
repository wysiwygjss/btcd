// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// fakeChecker is an in-memory BalanceChecker used to exercise Scan's
// gap-limit and concurrency logic without any network access. Addresses
// present in activity report that balance; every other address reports no
// activity. If failOn is set, looking up that exact address returns err
// instead.
type fakeChecker struct {
	mu       sync.Mutex
	calls    map[string]int
	activity map[string]AddressBalance
	failOn   string
	err      error
}

func newFakeChecker() *fakeChecker {
	return &fakeChecker{
		calls:    make(map[string]int),
		activity: make(map[string]AddressBalance),
	}
}

func (f *fakeChecker) Balance(ctx context.Context, address string) (AddressBalance, error) {
	f.mu.Lock()
	f.calls[address]++
	f.mu.Unlock()

	if f.failOn != "" && address == f.failOn {
		return AddressBalance{}, f.err
	}
	if b, ok := f.activity[address]; ok {
		return b, nil
	}
	return AddressBalance{}, nil
}

func (f *fakeChecker) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestScanStopsAtGapLimit checks that, with no activity anywhere, Scan
// checks exactly GapLimit addresses per branch and then stops.
func TestScanStopsAtGapLimit(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)
	scheme := allSchemes["84"]

	cfg := ScanConfig{
		Schemes:           []AddressScheme{scheme},
		Net:               &chaincfg.MainNetParams,
		Accounts:          1,
		ScanChange:        false,
		GapLimit:          3,
		MaxIndexPerBranch: 100,
		Concurrency:       2,
	}
	checker := newFakeChecker()

	results, err := Scan(context.Background(), master, cfg, checker, nil)
	if err != nil {
		t.Fatalf("Scan() returned unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("Scan() returned %d results, want 3 (the gap limit)",
			len(results))
	}
	for i, r := range results {
		if int(r.Index) != i {
			t.Errorf("result %d has index %d, want %d", i, r.Index, i)
		}
		if r.Balance.HasActivity() {
			t.Errorf("result %d unexpectedly has activity", i)
		}
	}
}

// TestScanContinuesPastActivity checks that finding activity resets the
// gap counter, so scanning continues GapLimit addresses past the last
// active one instead of stopping immediately.
func TestScanContinuesPastActivity(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)
	scheme := allSchemes["84"]
	net := &chaincfg.MainNetParams

	const activeIndex = 5
	activeAddr, err := deriveSchemeAddress(master, scheme, CoinType(net), 0,
		chainExternal, activeIndex, net)
	if err != nil {
		t.Fatalf("deriveSchemeAddress() returned unexpected error: %v", err)
	}

	checker := newFakeChecker()
	checker.activity[activeAddr] = AddressBalance{ConfirmedSats: 12345, TxCount: 1}

	cfg := ScanConfig{
		Schemes:           []AddressScheme{scheme},
		Net:               net,
		Accounts:          1,
		ScanChange:        false,
		GapLimit:          10,
		MaxIndexPerBranch: 100,
		Concurrency:       2,
	}

	results, err := Scan(context.Background(), master, cfg, checker, nil)
	if err != nil {
		t.Fatalf("Scan() returned unexpected error: %v", err)
	}

	// Since GapLimit (10) is comfortably larger than activeIndex (5),
	// the scan is guaranteed to reach the active address, reset its
	// gap counter there, and then continue for GapLimit more addresses.
	// Expect indices 0..activeIndex+GapLimit inclusive, i.e.
	// activeIndex+GapLimit+1 = 16 results.
	wantCount := activeIndex + int(cfg.GapLimit) + 1
	if len(results) != wantCount {
		t.Fatalf("Scan() returned %d results, want %d", len(results),
			wantCount)
	}

	var foundActive bool
	for _, r := range results {
		if r.Address == activeAddr {
			foundActive = true
			if !r.Balance.HasActivity() {
				t.Errorf("active address result reports no activity")
			}
		}
	}
	if !foundActive {
		t.Errorf("active address %s missing from results", activeAddr)
	}
}

// TestScanMultipleBranches checks that Scan derives a separate branch for
// every (scheme, account, chain) combination implied by the config.
func TestScanMultipleBranches(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)

	cfg := ScanConfig{
		Schemes:           []AddressScheme{allSchemes["44"], allSchemes["84"]},
		Net:               &chaincfg.MainNetParams,
		Accounts:          2,
		ScanChange:        true,
		GapLimit:          1,
		MaxIndexPerBranch: 100,
		Concurrency:       4,
	}
	checker := newFakeChecker()

	results, err := Scan(context.Background(), master, cfg, checker, nil)
	if err != nil {
		t.Fatalf("Scan() returned unexpected error: %v", err)
	}

	// 2 schemes * 2 accounts * 2 chains (external + change) = 8 branches,
	// each contributing exactly 1 result since GapLimit is 1 and nothing
	// has any activity.
	const wantBranches = 2 * 2 * 2
	if len(results) != wantBranches {
		t.Fatalf("Scan() returned %d results, want %d", len(results),
			wantBranches)
	}

	seen := make(map[string]bool)
	for _, r := range results {
		key := r.Path(CoinType(cfg.Net))
		if seen[key] {
			t.Errorf("duplicate branch/path seen: %s", key)
		}
		seen[key] = true
	}
}

// TestScanPropagatesError checks that a balance-lookup error is returned
// from Scan, without hanging or panicking.
func TestScanPropagatesError(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)
	scheme := allSchemes["84"]
	net := &chaincfg.MainNetParams

	failAddr, err := deriveSchemeAddress(master, scheme, CoinType(net), 0,
		chainExternal, 0, net)
	if err != nil {
		t.Fatalf("deriveSchemeAddress() returned unexpected error: %v", err)
	}

	checker := newFakeChecker()
	checker.failOn = failAddr
	checker.err = errors.New("simulated network failure")

	cfg := ScanConfig{
		Schemes:           []AddressScheme{scheme},
		Net:               net,
		Accounts:          1,
		ScanChange:        false,
		GapLimit:          20,
		MaxIndexPerBranch: 100,
		Concurrency:       1,
	}

	_, err = Scan(context.Background(), master, cfg, checker, nil)
	if err == nil {
		t.Fatalf("Scan() unexpectedly succeeded")
	}
}

// TestScanOnResultCallback checks that onResult is invoked once per
// derived address.
func TestScanOnResultCallback(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)

	cfg := ScanConfig{
		Schemes:           []AddressScheme{allSchemes["84"]},
		Net:               &chaincfg.MainNetParams,
		Accounts:          1,
		ScanChange:        false,
		GapLimit:          4,
		MaxIndexPerBranch: 100,
		Concurrency:       1,
	}
	checker := newFakeChecker()

	var (
		mu    sync.Mutex
		count int
	)
	_, err := Scan(context.Background(), master, cfg, checker, func(ScanResult) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("Scan() returned unexpected error: %v", err)
	}
	if count != 4 {
		t.Fatalf("onResult invoked %d times, want 4", count)
	}
}

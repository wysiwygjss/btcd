// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

// TestFormatReportNoActivity checks the report's message when nothing was
// found.
func TestFormatReportNoActivity(t *testing.T) {
	results := []ScanResult{
		{
			DerivedAddress: DerivedAddress{
				Scheme: allSchemes["84"], Account: 0, Chain: 0, Index: 0,
				Address: "bc1qexample",
			},
			Balance: AddressBalance{},
		},
	}

	report := FormatReport(results, 0, "mainnet", nil)
	if !strings.Contains(report, "No activity found") {
		t.Errorf("report missing 'no activity' message:\n%s", report)
	}
	if strings.Contains(report, "bc1qexample") {
		t.Errorf("report unexpectedly lists an inactive address:\n%s", report)
	}
}

// TestFormatReportWithActivity checks that active addresses, totals, and
// scan errors are all rendered.
func TestFormatReportWithActivity(t *testing.T) {
	results := []ScanResult{
		{
			DerivedAddress: DerivedAddress{
				Scheme: allSchemes["44"], Account: 0, Chain: 0, Index: 0,
				Address: "1Inactive",
			},
			Balance: AddressBalance{},
		},
		{
			DerivedAddress: DerivedAddress{
				Scheme: allSchemes["84"], Account: 0, Chain: 1, Index: 2,
				Address: "bc1qactive",
			},
			Balance: AddressBalance{ConfirmedSats: 5000, UnconfirmedSats: 100, TxCount: 2},
		},
	}

	report := FormatReport(results, 0, "mainnet", errTestScan)

	if !strings.Contains(report, "bc1qactive") {
		t.Errorf("report missing active address:\n%s", report)
	}
	if strings.Contains(report, "1Inactive") {
		t.Errorf("report unexpectedly lists an inactive address:\n%s", report)
	}
	if !strings.Contains(report, "5000") {
		t.Errorf("report missing confirmed balance:\n%s", report)
	}
	if !strings.Contains(report, errTestScan.Error()) {
		t.Errorf("report missing scan error message:\n%s", report)
	}
}

var errTestScan = &testError{"simulated scan failure"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestSortResultsOrder checks the canonical ordering: scheme purpose, then
// account, then chain, then index.
func TestSortResultsOrder(t *testing.T) {
	results := []ScanResult{
		{DerivedAddress: DerivedAddress{Scheme: allSchemes["86"], Account: 0, Chain: 0, Index: 0}},
		{DerivedAddress: DerivedAddress{Scheme: allSchemes["44"], Account: 1, Chain: 0, Index: 0}},
		{DerivedAddress: DerivedAddress{Scheme: allSchemes["44"], Account: 0, Chain: 1, Index: 0}},
		{DerivedAddress: DerivedAddress{Scheme: allSchemes["44"], Account: 0, Chain: 0, Index: 5}},
		{DerivedAddress: DerivedAddress{Scheme: allSchemes["44"], Account: 0, Chain: 0, Index: 0}},
	}

	SortResults(results)

	wantOrder := []uint32{44, 44, 44, 44, 86}
	for i, want := range wantOrder {
		if results[i].Scheme.Purpose != want {
			t.Fatalf("results[%d].Scheme.Purpose = %d, want %d", i,
				results[i].Scheme.Purpose, want)
		}
	}
	// Within purpose 44: (acct0,chain0,idx0) < (acct0,chain0,idx5) <
	// (acct0,chain1,idx0) < (acct1,chain0,idx0).
	first := results[0].DerivedAddress
	if first.Account != 0 || first.Chain != 0 || first.Index != 0 {
		t.Errorf("first result = %+v, want account/chain/index all 0", first)
	}
	last44 := results[3].DerivedAddress
	if last44.Account != 1 {
		t.Errorf("last purpose-44 result = %+v, want account 1", last44)
	}
}

// TestFormatAddressList checks the plain address-listing format used by
// -offline mode.
func TestFormatAddressList(t *testing.T) {
	addrs := []DerivedAddress{
		{Scheme: allSchemes["84"], Account: 0, Chain: 0, Index: 0, Address: "bc1qfoo"},
		{Scheme: allSchemes["84"], Account: 0, Chain: 0, Index: 1, Address: "bc1qbar"},
	}

	out := FormatAddressList(addrs, 0)
	if !strings.Contains(out, "m/84'/0'/0'/0/0") || !strings.Contains(out, "bc1qfoo") {
		t.Errorf("address list missing first entry:\n%s", out)
	}
	if !strings.Contains(out, "m/84'/0'/0'/0/1") || !strings.Contains(out, "bc1qbar") {
		t.Errorf("address list missing second entry:\n%s", out)
	}
}

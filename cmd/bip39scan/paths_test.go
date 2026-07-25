// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

// standardTestMnemonic is the canonical "all abandon" BIP39 test mnemonic
// used throughout BIP44/49/84/86's own specs for their worked examples,
// with an empty passphrase.
const standardTestMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

func mustMaster(t *testing.T, mnemonic string, net *chaincfg.Params) *hdkeychain.ExtendedKey {
	t.Helper()

	seed, err := SeedFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("SeedFromMnemonic() returned unexpected error: %v", err)
	}
	master, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		t.Fatalf("NewMaster() returned unexpected error: %v", err)
	}
	return master
}

// TestDerivePathAndSchemesAgainstOfficialVectors checks address derivation
// for every supported scheme against the worked examples published in each
// BIP's own specification (BIP44, BIP49, BIP84, and BIP86), all of which
// use standardTestMnemonic.
func TestDerivePathAndSchemesAgainstOfficialVectors(t *testing.T) {
	testCases := []struct {
		name    string
		purpose uint32
		net     *chaincfg.Params
		account uint32
		chain   uint32
		index   uint32
		want    string
	}{
		{
			name:    "BIP44 legacy P2PKH, mainnet, first receive",
			purpose: 44,
			net:     &chaincfg.MainNetParams,
			want:    "1LqBGSKuX5yYUonjxT5qGfpUsXKYYWeabA",
		},
		{
			name:    "BIP49 nested SegWit P2SH-P2WPKH, testnet, first receive",
			purpose: 49,
			net:     &chaincfg.TestNet3Params,
			want:    "2Mww8dCYPUpKHofjgcXcBCEGmniw9CoaiD2",
		},
		{
			name:    "BIP84 native SegWit P2WPKH, mainnet, first receive",
			purpose: 84,
			net:     &chaincfg.MainNetParams,
			want:    "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu",
		},
		{
			name:    "BIP84 native SegWit P2WPKH, mainnet, second receive",
			purpose: 84,
			net:     &chaincfg.MainNetParams,
			index:   1,
			want:    "bc1qnjg0jd8228aq7egyzacy8cys3knf9xvrerkf9g",
		},
		{
			name:    "BIP84 native SegWit P2WPKH, mainnet, first change",
			purpose: 84,
			net:     &chaincfg.MainNetParams,
			chain:   chainInternal,
			want:    "bc1q8c6fshw2dlwun7ekn9qwf37cu2rn755upcp6el",
		},
		{
			name:    "BIP86 taproot P2TR, mainnet, first receive",
			purpose: 86,
			net:     &chaincfg.MainNetParams,
			want:    "bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			master := mustMaster(t, standardTestMnemonic, tc.net)

			scheme, ok := allSchemes[schemeKeyForPurpose(tc.purpose)]
			if !ok {
				t.Fatalf("no scheme registered for purpose %d", tc.purpose)
			}

			addr, err := deriveSchemeAddress(master, scheme, CoinType(tc.net),
				tc.account, tc.chain, tc.index, tc.net)
			if err != nil {
				t.Fatalf("deriveSchemeAddress() returned unexpected "+
					"error: %v", err)
			}
			if addr != tc.want {
				t.Fatalf("address mismatch:\ngot:  %s\nwant: %s", addr,
					tc.want)
			}
		})
	}
}

func schemeKeyForPurpose(purpose uint32) string {
	for key, scheme := range allSchemes {
		if scheme.Purpose == purpose {
			return key
		}
	}
	return ""
}

// TestCoinType checks the mainnet-vs-testnet coin type convention.
func TestCoinType(t *testing.T) {
	if got := CoinType(&chaincfg.MainNetParams); got != 0 {
		t.Fatalf("CoinType(mainnet) = %d, want 0", got)
	}
	if got := CoinType(&chaincfg.TestNet3Params); got != 1 {
		t.Fatalf("CoinType(testnet3) = %d, want 1", got)
	}
}

// TestParseSchemes checks scheme-list parsing, including the "all" default
// and canonical ordering regardless of input order.
func TestParseSchemes(t *testing.T) {
	testCases := []struct {
		name    string
		spec    string
		want    []string
		wantErr bool
	}{
		{name: "empty defaults to all", spec: "", want: []string{"44", "49", "84", "86"}},
		{name: "explicit all", spec: "all", want: []string{"44", "49", "84", "86"}},
		{name: "single scheme", spec: "84", want: []string{"84"}},
		{name: "reordered input still canonical order", spec: "86,44", want: []string{"44", "86"}},
		{name: "whitespace tolerated", spec: " 44 , 84 ", want: []string{"44", "84"}},
		{name: "duplicate keys collapse", spec: "44,44", want: []string{"44"}},
		{name: "unknown scheme", spec: "99", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSchemes(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSchemes(%q) unexpectedly succeeded",
						tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSchemes(%q) returned unexpected error: %v",
					tc.spec, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseSchemes(%q) = %d schemes, want %d",
					tc.spec, len(got), len(tc.want))
			}
			for i, scheme := range got {
				if scheme.Key != tc.want[i] {
					t.Fatalf("ParseSchemes(%q)[%d].Key = %q, want %q",
						tc.spec, i, scheme.Key, tc.want[i])
				}
			}
		})
	}
}

// TestDerivePathIsDeterministic checks that deriving the same path twice
// from the same master key yields identical keys, and that different
// indices yield different keys.
func TestDerivePathIsDeterministic(t *testing.T) {
	master := mustMaster(t, standardTestMnemonic, &chaincfg.MainNetParams)

	k1, err := DerivePath(master, 84, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("DerivePath() returned unexpected error: %v", err)
	}
	k2, err := DerivePath(master, 84, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("DerivePath() returned unexpected error: %v", err)
	}
	if k1.String() != k2.String() {
		t.Fatalf("deriving the same path twice gave different keys")
	}

	k3, err := DerivePath(master, 84, 0, 0, 0, 1)
	if err != nil {
		t.Fatalf("DerivePath() returned unexpected error: %v", err)
	}
	if k1.String() == k3.String() {
		t.Fatalf("deriving different indices gave the same key")
	}
}

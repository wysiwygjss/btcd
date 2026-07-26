// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// SortResults sorts results in a stable, human-friendly order: by scheme
// (BIP44, then 49, then 84, then 86), then account, then chain
// (external before internal), then address index.
func SortResults(results []ScanResult) {
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Scheme.Purpose != b.Scheme.Purpose {
			return a.Scheme.Purpose < b.Scheme.Purpose
		}
		if a.Account != b.Account {
			return a.Account < b.Account
		}
		if a.Chain != b.Chain {
			return a.Chain < b.Chain
		}
		return a.Index < b.Index
	})
}

// chainLabel renders a BIP44 chain value ("0"/"1") as a short human label.
func chainLabel(chain uint32) string {
	if chain == chainInternal {
		return "change"
	}
	return "receive"
}

// FormatReport renders a completed scan's results as a plain-text report:
// a summary of how many addresses were checked and how much value was
// found, followed by a table of every address with any on-chain activity.
func FormatReport(
	results []ScanResult,
	coinType uint32,
	networkName string,
	scanErr error,
) string {

	sorted := append([]ScanResult(nil), results...)
	SortResults(sorted)

	var totalConfirmed, totalUnconfirmed int64
	var withActivity int
	for _, r := range sorted {
		if r.Balance.HasActivity() {
			withActivity++
			totalConfirmed += r.Balance.ConfirmedSats
			totalUnconfirmed += r.Balance.UnconfirmedSats
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "bip39scan report\n")
	fmt.Fprintf(&sb, "Generated:                 %s\n",
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(&sb, "Network:                    %s\n", networkName)
	fmt.Fprintf(&sb, "Addresses checked:          %d\n", len(sorted))
	fmt.Fprintf(&sb, "Addresses with activity:    %d\n", withActivity)
	fmt.Fprintf(&sb, "Total confirmed balance:    %d sats\n", totalConfirmed)
	fmt.Fprintf(&sb, "Total unconfirmed balance:  %d sats\n", totalUnconfirmed)
	if scanErr != nil {
		fmt.Fprintf(&sb, "\nWARNING: scan ended early due to an error, so "+
			"results above are incomplete:\n  %v\n", scanErr)
	}
	sb.WriteString("\n")

	if withActivity == 0 {
		sb.WriteString("No activity found on any scanned address.\n")
		return sb.String()
	}

	fmt.Fprintf(&sb, "%-36s %-4s %-8s %-6s %-42s %14s %14s %5s\n",
		"Scheme", "Acct", "Branch", "Index", "Address", "Confirmed",
		"Unconfirmed", "Txs")
	for _, r := range sorted {
		if !r.Balance.HasActivity() {
			continue
		}
		fmt.Fprintf(&sb, "%-36s %-4d %-8s %-6d %-42s %14d %14d %5d\n",
			r.Scheme.Label, r.Account, chainLabel(r.Chain), r.Index,
			r.Address, r.Balance.ConfirmedSats, r.Balance.UnconfirmedSats,
			r.Balance.TxCount)
	}

	return sb.String()
}

// FormatAddressList renders a list of derived addresses (with no balance
// information), one per line as "<path>  <address>", for offline/dry-run
// use where no network calls are made.
func FormatAddressList(addrs []DerivedAddress, coinType uint32) string {
	var sb strings.Builder
	for _, a := range addrs {
		fmt.Fprintf(&sb, "%-24s %s\n", a.Path(coinType), a.Address)
	}
	return sb.String()
}

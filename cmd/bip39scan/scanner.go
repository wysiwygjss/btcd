// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

// chainExternal and chainInternal are the two BIP44 "chain" branches: 0 for
// receive (external) addresses, 1 for change (internal) addresses.
const (
	chainExternal = 0
	chainInternal = 1
)

// ScanConfig configures a multi-path balance scan.
type ScanConfig struct {
	// Schemes is the set of BIP44-family derivation schemes to scan,
	// e.g. BIP44/49/84/86.
	Schemes []AddressScheme

	// Net selects which network's address encoding and default coin
	// type to use.
	Net *chaincfg.Params

	// Accounts is how many accounts (m/purpose'/coin'/0' through
	// m/purpose'/coin'/(Accounts-1)') to scan for each scheme.
	Accounts uint32

	// ScanChange additionally scans the internal/change chain
	// (m/.../1/*) for each account, not just the external/receive chain
	// (m/.../0/*).
	ScanChange bool

	// GapLimit is how many consecutive addresses without any on-chain
	// activity must be seen within one (scheme, account, chain) branch
	// before bip39scan stops scanning further indices on that branch,
	// mirroring the gap limit convention used by wallets such as
	// Electrum. Must be at least 1.
	GapLimit uint32

	// MaxIndexPerBranch is a hard safety cap on how many indices are
	// ever checked within a single branch, regardless of GapLimit. This
	// bounds the work done for a single scan even if activity keeps
	// appearing right up against the gap limit indefinitely.
	MaxIndexPerBranch uint32

	// Concurrency is the maximum number of balance lookups allowed to
	// be in flight at once, across every branch being scanned.
	Concurrency int
}

// DerivedAddress identifies exactly where in the derivation tree an address
// came from.
type DerivedAddress struct {
	Scheme  AddressScheme
	Account uint32
	Chain   uint32
	Index   uint32
	Address string
}

// Path renders the address's location as a standard derivation path string,
// e.g. "m/84'/0'/0'/0/5".
func (d DerivedAddress) Path(coinType uint32) string {
	return fmt.Sprintf("m/%d'/%d'/%d'/%d/%d", d.Scheme.Purpose, coinType,
		d.Account, d.Chain, d.Index)
}

// ScanResult pairs a derived address with the on-chain activity found for
// it.
type ScanResult struct {
	DerivedAddress
	Balance AddressBalance
}

// Scan derives addresses along every (scheme, account, chain) branch
// implied by cfg and queries checker for each one's balance, stopping each
// branch once cfg.GapLimit consecutive addresses show no activity (or
// cfg.MaxIndexPerBranch is reached). Branches are scanned concurrently, up
// to cfg.Concurrency balance lookups in flight at a time; onResult, if
// non-nil, is invoked (from possibly many goroutines) as soon as each
// address's balance is known, in addition to it being included in the
// returned slice.
//
// Scan returns every result gathered before ctx was canceled or a
// non-retryable error occurred, together with that error (nil on a clean
// completion).
func Scan(
	ctx context.Context,
	master *hdkeychain.ExtendedKey,
	cfg ScanConfig,
	checker BalanceChecker,
	onResult func(ScanResult),
) ([]ScanResult, error) {

	if cfg.GapLimit == 0 {
		cfg.GapLimit = 1
	}
	if cfg.MaxIndexPerBranch == 0 {
		cfg.MaxIndexPerBranch = 1000
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	coinType := CoinType(cfg.Net)

	type branch struct {
		scheme  AddressScheme
		account uint32
		chain   uint32
	}
	var branches []branch
	for _, scheme := range cfg.Schemes {
		for account := uint32(0); account < cfg.Accounts; account++ {
			branches = append(branches, branch{scheme, account, chainExternal})
			if cfg.ScanChange {
				branches = append(branches, branch{scheme, account, chainInternal})
			}
		}
	}

	sem := make(chan struct{}, cfg.Concurrency)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		results  []ScanResult
		firstErr error
	)

	recordErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}

	for _, b := range branches {
		b := b
		wg.Add(1)
		go func() {
			defer wg.Done()

			consecutiveEmpty := uint32(0)
			for index := uint32(0); index < cfg.MaxIndexPerBranch; index++ {
				if ctx.Err() != nil {
					return
				}

				// hdkeychain.ExtendedKey.Derive lazily memoizes each
				// key's serialized public key on first use, mutating
				// the receiver; since every branch starts walking down
				// from the very same shared master key, concurrent
				// derivations must be serialized. This only guards the
				// (fast, local) key-derivation step, not the (slow,
				// network-bound) balance lookup below.
				mu.Lock()
				addr, err := deriveSchemeAddress(master, b.scheme, coinType,
					b.account, b.chain, index, cfg.Net)
				mu.Unlock()
				if err != nil {
					recordErr(fmt.Errorf("failed to derive %s account "+
						"%d chain %d index %d: %w", b.scheme.Label,
						b.account, b.chain, index, err))
					return
				}

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				balance, err := checker.Balance(ctx, addr)
				<-sem

				if err != nil {
					if ctx.Err() != nil {
						return
					}
					recordErr(fmt.Errorf("failed to check balance for "+
						"%s: %w", addr, err))
					return
				}

				result := ScanResult{
					DerivedAddress: DerivedAddress{
						Scheme:  b.scheme,
						Account: b.account,
						Chain:   b.chain,
						Index:   index,
						Address: addr,
					},
					Balance: balance,
				}

				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				if onResult != nil {
					onResult(result)
				}

				if balance.HasActivity() {
					consecutiveEmpty = 0
				} else {
					consecutiveEmpty++
					if consecutiveEmpty >= cfg.GapLimit {
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	return results, firstErr
}

// deriveSchemeAddress walks master down to (scheme, account, chain, index)
// and encodes the resulting public key using scheme's address type.
func deriveSchemeAddress(
	master *hdkeychain.ExtendedKey,
	scheme AddressScheme,
	coinType, account, chain, index uint32,
	net *chaincfg.Params,
) (string, error) {

	key, err := DerivePath(master, scheme.Purpose, coinType, account, chain,
		index)
	if err != nil {
		return "", err
	}

	pubKey, err := key.ECPubKey()
	if err != nil {
		return "", fmt.Errorf("failed to get public key: %w", err)
	}

	addr, err := scheme.deriveAddress(pubKey, net)
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}

	return addr.EncodeAddress(), nil
}

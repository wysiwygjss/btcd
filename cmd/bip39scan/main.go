// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// bip39scan derives Bitcoin addresses from a BIP39 mnemonic seed phrase
// along every standard BIP44-family derivation path (legacy P2PKH via
// BIP44, nested SegWit P2SH-P2WPKH via BIP49, native SegWit P2WPKH via
// BIP84, and Taproot P2TR via BIP86) across as many accounts and address
// indices as requested, and checks each derived address's on-chain balance
// against an Esplora-compatible REST API.
//
// It exists to answer "does this seed phrase control any funds, and if so,
// on which of the many paths and address types wallets commonly use?" —
// useful when recovering an old or unfamiliar wallet backup whose exact
// derivation scheme is unknown.
//
// Only the addresses bip39scan derives are ever sent over the network
// (to look up their balance); the mnemonic, passphrase, seed, and all
// derived keys always stay local. Pass -offline to skip network access
// entirely and just list the addresses that would have been checked.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

type config struct {
	mnemonic     string
	mnemonicFile string
	passphrase   string
	seedHex      string
	testnet      bool
	schemes      string
	accounts     uint
	scanChange   bool
	gapLimit     uint
	maxIndex     uint
	concurrency  int
	timeout      time.Duration
	apiBase      string
	maxRetries   int
	offline      bool
	quiet        bool
	outPath      string
}

func parseFlags() *config {
	cfg := &config{}

	flag.StringVar(&cfg.mnemonic, "mnemonic", "",
		"BIP39 mnemonic seed phrase (space-separated words). Prefer "+
			"-mnemonic-file when possible to avoid leaving the phrase in "+
			"your shell history or process list")
	flag.StringVar(&cfg.mnemonicFile, "mnemonic-file", "",
		"path to a file containing the BIP39 mnemonic seed phrase")
	flag.StringVar(&cfg.passphrase, "passphrase", "",
		"optional BIP39 passphrase (the \"25th word\")")
	flag.StringVar(&cfg.seedHex, "seed-hex", "",
		"use this hex-encoded seed directly instead of deriving one from "+
			"a mnemonic (16-64 bytes / 32-128 hex characters)")
	flag.BoolVar(&cfg.testnet, "testnet", false,
		"derive addresses for testnet3 instead of mainnet")
	flag.StringVar(&cfg.schemes, "schemes", "all",
		"comma-separated derivation schemes to scan: 44, 49, 84, 86, or "+
			"all")
	flag.UintVar(&cfg.accounts, "accounts", 1,
		"number of accounts to scan per scheme (m/.../0' through "+
			"m/.../(accounts-1)')")
	flag.BoolVar(&cfg.scanChange, "scan-change", true,
		"also scan each account's internal/change chain (m/.../1/*), not "+
			"just its external/receive chain (m/.../0/*)")
	flag.UintVar(&cfg.gapLimit, "gap-limit", 20,
		"stop scanning a branch after this many consecutive addresses "+
			"show no on-chain activity")
	flag.UintVar(&cfg.maxIndex, "max-index", 1000,
		"hard cap on how many addresses are ever checked per branch, "+
			"regardless of -gap-limit")
	flag.IntVar(&cfg.concurrency, "concurrency", 4,
		"maximum number of balance lookups in flight at once")
	flag.DurationVar(&cfg.timeout, "timeout", 15*time.Second,
		"per-request timeout for balance lookups")
	flag.StringVar(&cfg.apiBase, "api-base", "",
		"base URL of an Esplora-compatible REST API to query for "+
			"balances (default: blockstream.info's public API for the "+
			"selected network). Point this at a self-hosted instance to "+
			"avoid revealing derived addresses to a third party")
	flag.IntVar(&cfg.maxRetries, "max-retries", 3,
		"how many times to retry a balance lookup after a transient "+
			"failure (network error, HTTP 429, or HTTP 5xx)")
	flag.BoolVar(&cfg.offline, "offline", false,
		"skip balance checks entirely; only derive and list the "+
			"addresses that would have been checked (up to -gap-limit "+
			"addresses per branch)")
	flag.BoolVar(&cfg.quiet, "quiet", false,
		"suppress per-address progress logging; only print the final "+
			"report")
	flag.StringVar(&cfg.outPath, "out", "",
		"also write the final report to this file")

	flag.Parse()

	return cfg
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func run(cfg *config) error {
	net := &chaincfg.MainNetParams
	networkName := "mainnet"
	if cfg.testnet {
		net = &chaincfg.TestNet3Params
		networkName = "testnet3"
	}

	seed, err := resolveSeed(cfg)
	if err != nil {
		return err
	}

	master, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		return fmt.Errorf("failed to derive master key: %w", err)
	}
	defer master.Zero()

	schemes, err := ParseSchemes(cfg.schemes)
	if err != nil {
		return err
	}

	if cfg.accounts == 0 {
		return fmt.Errorf("-accounts must be at least 1")
	}
	if cfg.gapLimit == 0 {
		return fmt.Errorf("-gap-limit must be at least 1")
	}

	coinType := CoinType(net)

	if cfg.offline {
		return runOffline(master, schemes, coinType, net, cfg)
	}

	return runOnline(master, schemes, coinType, net, networkName, cfg)
}

// resolveSeed obtains the raw BIP32 seed to use, either directly from
// -seed-hex or by deriving it from a BIP39 mnemonic and passphrase.
func resolveSeed(cfg *config) ([]byte, error) {
	if cfg.seedHex != "" {
		seed, err := hex.DecodeString(strings.TrimSpace(cfg.seedHex))
		if err != nil {
			return nil, fmt.Errorf("failed to decode -seed-hex: %w", err)
		}
		if len(seed) < hdkeychain.MinSeedBytes ||
			len(seed) > hdkeychain.MaxSeedBytes {

			return nil, fmt.Errorf("-seed-hex must decode to between %d "+
				"and %d bytes, got %d", hdkeychain.MinSeedBytes,
				hdkeychain.MaxSeedBytes, len(seed))
		}
		return seed, nil
	}

	mnemonic := cfg.mnemonic
	if cfg.mnemonicFile != "" {
		if mnemonic != "" {
			return nil, fmt.Errorf("specify only one of -mnemonic, " +
				"-mnemonic-file, or -seed-hex")
		}
		raw, err := os.ReadFile(cfg.mnemonicFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read -mnemonic-file: %w", err)
		}
		mnemonic = strings.TrimSpace(string(raw))
	}

	if mnemonic == "" {
		return nil, fmt.Errorf("one of -mnemonic, -mnemonic-file, or " +
			"-seed-hex is required")
	}

	seed, err := SeedFromMnemonic(mnemonic, cfg.passphrase)
	if err != nil {
		return nil, fmt.Errorf("invalid mnemonic: %w", err)
	}
	return seed, nil
}

// runOffline derives (but does not check the balance of) up to -gap-limit
// addresses per branch, and prints/saves the resulting address list.
func runOffline(
	master *hdkeychain.ExtendedKey,
	schemes []AddressScheme,
	coinType uint32,
	net *chaincfg.Params,
	cfg *config,
) error {

	var addrs []DerivedAddress
	for _, scheme := range schemes {
		for account := uint32(0); account < uint32(cfg.accounts); account++ {
			chains := []uint32{chainExternal}
			if cfg.scanChange {
				chains = append(chains, chainInternal)
			}
			for _, chain := range chains {
				for index := uint32(0); index < uint32(cfg.gapLimit); index++ {
					addr, err := deriveSchemeAddress(master, scheme,
						coinType, account, chain, index, net)
					if err != nil {
						return fmt.Errorf("failed to derive address: %w",
							err)
					}
					addrs = append(addrs, DerivedAddress{
						Scheme:  scheme,
						Account: account,
						Chain:   chain,
						Index:   index,
						Address: addr,
					})
				}
			}
		}
	}

	report := FormatAddressList(addrs, coinType)
	fmt.Print(report)

	return maybeSaveReport(cfg.outPath, report)
}

// runOnline runs a full multi-path balance scan against a live balance API
// and prints/saves the resulting report.
func runOnline(
	master *hdkeychain.ExtendedKey,
	schemes []AddressScheme,
	coinType uint32,
	net *chaincfg.Params,
	networkName string,
	cfg *config,
) error {

	apiBase := cfg.apiBase
	if apiBase == "" {
		apiBase = defaultAPIBase(net)
	}

	checker := &EsploraClient{
		BaseURL:        apiBase,
		Timeout:        cfg.timeout,
		MaxRetries:     cfg.maxRetries,
		RetryBaseDelay: time.Second,
	}

	scanCfg := ScanConfig{
		Schemes:           schemes,
		Net:               net,
		Accounts:          uint32(cfg.accounts),
		ScanChange:        cfg.scanChange,
		GapLimit:          uint32(cfg.gapLimit),
		MaxIndexPerBranch: uint32(cfg.maxIndex),
		Concurrency:       cfg.concurrency,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt,
		syscall.SIGTERM)
	defer cancel()

	log.Printf("scanning %d scheme(s) x %d account(s) against %s ...",
		len(schemes), cfg.accounts, apiBase)

	onResult := func(r ScanResult) {
		if cfg.quiet {
			return
		}
		if r.Balance.HasActivity() {
			log.Printf("%-36s %-24s %s  confirmed=%d sats  "+
				"unconfirmed=%d sats  txs=%d  *** ACTIVITY ***",
				r.Scheme.Label, r.Path(coinType), r.Address,
				r.Balance.ConfirmedSats, r.Balance.UnconfirmedSats,
				r.Balance.TxCount)
		} else {
			log.Printf("%-36s %-24s %s  (no activity)", r.Scheme.Label,
				r.Path(coinType), r.Address)
		}
	}

	results, scanErr := Scan(ctx, master, scanCfg, checker, onResult)
	if scanErr != nil {
		log.Printf("scan stopped early: %v", scanErr)
	}

	report := FormatReport(results, coinType, networkName, scanErr)
	fmt.Print("\n" + report)

	if err := maybeSaveReport(cfg.outPath, report); err != nil {
		return err
	}

	return scanErr
}

// defaultAPIBase picks a sensible public Esplora API base URL for net.
func defaultAPIBase(net *chaincfg.Params) string {
	if net.Net == chaincfg.MainNetParams.Net {
		return "https://blockstream.info/api"
	}
	return "https://blockstream.info/testnet/api"
}

// maybeSaveReport writes report to path if path is non-empty.
func maybeSaveReport(path, report string) error {
	if path == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		return fmt.Errorf("failed to write report to %s: %w", path, err)
	}
	log.Printf("report saved to %s", path)
	return nil
}

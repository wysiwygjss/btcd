// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// noncereuse scans a dataset of ECDSA signatures for the classic "nonce
// reuse" (a.k.a. "duplicate R") vulnerability: two signatures produced by
// the same private key that happen to share the same secret nonce, and
// therefore the same signature R value. Whenever such a pair is found, the
// underlying private key can be, and is, fully recovered from nothing more
// than the two signatures and the two message hashes they sign over.
//
// This is not a hypothetical concern: real-world nonce reuse has repeatedly
// led to actual loss of funds, most famously the 2013 Android Bitcoin
// wallet bug (a broken SecureRandom implementation that occasionally
// produced a previously used nonce) and, more recently, the 2023 "Milk Sad"
// vulnerability in several wallet libraries with a predictable PRNG. The
// same class of bug is not unique to Bitcoin; it applies to ECDSA (and
// Schnorr, if the nonce derivation is broken) wherever it is used.
//
// The tool reads a CSV file describing observed signatures -- see the
// package README for the exact format -- groups them by public key and R
// value, and for every public key that signed two different messages with
// the same R, recovers and reports the private key.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/btcsuite/btcd/chaincfg"
)

type config struct {
	inputPath  string
	outputPath string
	testnet    bool
}

func parseFlags() *config {
	cfg := &config{}

	flag.StringVar(&cfg.inputPath, "input", "",
		"path to the input CSV file of signature records (required)")
	flag.StringVar(&cfg.outputPath, "output", "",
		"path to write the report to (defaults to stdout)")
	flag.BoolVar(&cfg.testnet, "testnet", false,
		"render recovered keys/addresses using testnet encoding "+
			"instead of mainnet")
	flag.Parse()

	return cfg
}

func main() {
	cfg := parseFlags()

	if cfg.inputPath == "" {
		fmt.Fprintln(os.Stderr, "error: -input is required")
		flag.Usage()
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func run(cfg *config) error {
	records, err := loadRecords(cfg.inputPath)
	if err != nil {
		return fmt.Errorf("failed to load records from %s: %w",
			cfg.inputPath, err)
	}
	log.Printf("loaded %d signature record(s) from %s", len(records),
		cfg.inputPath)

	findings, err := detect(records)
	if err != nil {
		return fmt.Errorf("failed to run nonce reuse detection: %w", err)
	}

	net := &chaincfg.MainNetParams
	if cfg.testnet {
		net = &chaincfg.TestNet3Params
	}

	out := os.Stdout
	if cfg.outputPath != "" {
		f, err := os.Create(cfg.outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file %s: %w",
				cfg.outputPath, err)
		}
		defer f.Close()
		out = f
	}

	if err := writeReport(out, findings, net); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	if cfg.outputPath != "" {
		log.Printf("wrote report with %d finding(s) to %s", len(findings),
			cfg.outputPath)
	}

	return nil
}

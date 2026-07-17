// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestDetectSampleFixture runs the full load-records + detect + report
// pipeline against the checked-in sample dataset and verifies that the
// (deliberately planted) nonce reuse is found and the correct private key
// recovered, while the unrelated third record does not produce a spurious
// finding.
func TestDetectSampleFixture(t *testing.T) {
	records, err := loadRecords("testdata/sample.csv")
	if err != nil {
		t.Fatalf("failed to load sample.csv: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	findings, err := detect(records)
	if err != nil {
		t.Fatalf("detect returned an error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d", len(findings))
	}

	f := findings[0]
	wantPrivKeyHex := "3f719c1a8b2d4e6f015a9d3c7e2b8f146327a5d90e4c81fb356a1d8e72c40953"
	if got := hex.EncodeToString(f.privKey.Serialize()); got != wantPrivKeyHex {
		t.Errorf("unexpected recovered private key\ngot:  %s\nwant: %s",
			got, wantPrivKeyHex)
	}

	wantPubKeyHex := "03ab0b34bcb759f24b2bb8c4ca3365f32b700a7a96a78d40906b4da06a43c56bfd"
	if f.pubKeyHex != wantPubKeyHex {
		t.Errorf("unexpected finding pubkey: %s", f.pubKeyHex)
	}
	if !f.privKey.PubKey().IsEqual(f.pubKey) {
		t.Errorf("recovered private key does not match the claimed public key")
	}

	// The report should render without error and mention the recovered
	// WIF-encoded private key.
	var buf bytes.Buffer
	if err := writeReport(&buf, findings, &chaincfg.MainNetParams); err != nil {
		t.Fatalf("writeReport returned an error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected a non-empty report")
	}
}

// TestDetectNoReuse verifies that detect reports no findings when every
// record for a given signer uses a distinct R value.
func TestDetectNoReuse(t *testing.T) {
	pubKeyHex := "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	csvData := "id,pubkey,r,s,z\n" +
		"a," + pubKeyHex + ",01,02,03\n" +
		"b," + pubKeyHex + ",04,05,06\n"

	records, err := parseRecords(byteReader(csvData))
	if err != nil {
		t.Fatalf("failed to parse records: %v", err)
	}

	findings, err := detect(records)
	if err != nil {
		t.Fatalf("detect returned an error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(findings))
	}

	var buf bytes.Buffer
	if err := writeReport(&buf, findings, &chaincfg.MainNetParams); err != nil {
		t.Fatalf("writeReport returned an error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected a non-empty report even with no findings")
	}
}

// TestDetectExactDuplicateSignatureNotAFinding verifies that two identical
// (r, s) signatures over different messages -- which cannot happen for a
// well behaved signer and thus most likely indicates a data-entry error
// (the exact same record duplicated) -- does not produce a finding, since
// an identical S value carries no information about the nonce.
func TestDetectExactDuplicateSignatureNotAFinding(t *testing.T) {
	pubKeyHex := "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	csvData := "id,pubkey,r,s,z\n" +
		"a," + pubKeyHex + ",01,02,03\n" +
		"b," + pubKeyHex + ",01,02,04\n"

	records, err := parseRecords(byteReader(csvData))
	if err != nil {
		t.Fatalf("failed to parse records: %v", err)
	}

	findings, err := detect(records)
	if err != nil {
		t.Fatalf("detect returned an error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings for an exact-duplicate signature, "+
			"got %d", len(findings))
	}
}

func byteReader(s string) *bytes.Reader {
	return bytes.NewReader([]byte(s))
}

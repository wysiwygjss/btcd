// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestParseRecordsRSColumns(t *testing.T) {
	csvData := "id,pubkey,r,s,z\n" +
		"rec-a,03ab0b34bcb759f24b2bb8c4ca3365f32b700a7a96a78d40906b4da06a43c56bfd," +
		"fa19b5301b0d38ba2b3017643cba28dd8d0e12f49f3db3fd2729ecb183c99971," +
		"7c7e422f542a753c27864503717408b89596084ac15783db1522cdfb50b5a536," +
		"a0078a43a40270261a665760cfac7c4a48ac40a4aebacdddd96c187ae0920278\n"

	records, err := parseRecords(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.id != "rec-a" {
		t.Errorf("unexpected id: %s", rec.id)
	}
	if got := hex.EncodeToString(rec.r[:]); got !=
		"fa19b5301b0d38ba2b3017643cba28dd8d0e12f49f3db3fd2729ecb183c99971" {
		t.Errorf("unexpected r: %s", got)
	}
	if got := hex.EncodeToString(rec.s[:]); got !=
		"7c7e422f542a753c27864503717408b89596084ac15783db1522cdfb50b5a536" {
		t.Errorf("unexpected s: %s", got)
	}
	wantZ := "a0078a43a40270261a665760cfac7c4a48ac40a4aebacdddd96c187ae0920278"
	if got := hex.EncodeToString(rec.z); got != wantZ {
		t.Errorf("unexpected z: %s", got)
	}
}

func TestParseRecordsSigColumn(t *testing.T) {
	// A DER-encoded signature (r, s) with r and s as small, easy to
	// recognize values, followed by an arbitrary z. This exercises the
	// "sig" column path, which round-trips through
	// ecdsa.ComponentsFromSignature under the hood.
	csvData := "id,pubkey,sig,z\n" +
		"rec-a,0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798," +
		"3006020101020102," +
		"aa\n"

	records, err := parseRecords(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	wantR := make([]byte, 32)
	wantR[31] = 1
	wantS := make([]byte, 32)
	wantS[31] = 2
	if hex.EncodeToString(rec.r[:]) != hex.EncodeToString(wantR) {
		t.Errorf("unexpected r: %x", rec.r)
	}
	if hex.EncodeToString(rec.s[:]) != hex.EncodeToString(wantS) {
		t.Errorf("unexpected s: %x", rec.s)
	}
}

func TestParseRecordsMissingColumns(t *testing.T) {
	tests := []struct {
		name    string
		csvData string
	}{
		{
			name:    "missing pubkey column",
			csvData: "id,r,s,z\na,1,2,3\n",
		},
		{
			name:    "missing z column",
			csvData: "id,pubkey,r,s\na,02,1,2\n",
		},
		{
			name:    "missing sig and r/s columns",
			csvData: "id,pubkey,z\na,02,3\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseRecords(strings.NewReader(test.csvData))
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
		})
	}
}

func TestParseRecordsCommentsAndBlankHandling(t *testing.T) {
	csvData := "id,pubkey,r,s,z\n" +
		"# this is a comment and should be skipped,ignored,ignored,ignored,ignored\n" +
		"rec-a,0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798," +
		"01,02,03\n"

	records, err := parseRecords(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (comment line skipped), got %d",
			len(records))
	}
}

// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunEndToEnd exercises run (the same code main invokes) against the
// checked-in sample dataset and verifies that a report file is written
// containing the expected recovered private key.
func TestRunEndToEnd(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "report.txt")

	cfg := &config{
		inputPath:  "testdata/sample.csv",
		outputPath: outPath,
	}
	if err := run(cfg); err != nil {
		t.Fatalf("run returned an error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	report := string(data)
	wantPrivKeyHex := "3f719c1a8b2d4e6f015a9d3c7e2b8f146327a5d90e4c81fb356a1d8e72c40953"
	if !strings.Contains(report, wantPrivKeyHex) {
		t.Fatalf("report does not contain the expected recovered private "+
			"key %q:\n%s", wantPrivKeyHex, report)
	}
}

// TestRunMissingInput verifies that run surfaces a clear error when the
// input file does not exist.
func TestRunMissingInput(t *testing.T) {
	cfg := &config{
		inputPath: filepath.Join(t.TempDir(), "does-not-exist.csv"),
	}
	if err := run(cfg); err == nil {
		t.Fatalf("expected an error for a missing input file")
	}
}

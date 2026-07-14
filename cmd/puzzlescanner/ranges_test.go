// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ranges.txt")
	contents := "" +
		"0000000000000000000000000000000000000000000000400000000000000000:00000000000000000000000000000000000000000000004001a36e2eb1c432c9\n" +
		"\n" + // blank lines should be skipped
		"00000000000000000000000000000000000000000000004001a36e2eb1c432ca:0000000000000000000000000000000000000000000000400346dc5d63886593\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write test ranges file: %v", err)
	}

	ranges, err := loadRanges(path)
	if err != nil {
		t.Fatalf("loadRanges returned error: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}

	if ranges[0].ID != 1 || ranges[1].ID != 3 {
		t.Fatalf("expected chunk IDs 1 and 3 (by line number), got %d and %d",
			ranges[0].ID, ranges[1].ID)
	}

	wantStart, _ := new(big.Int).SetString(
		"400000000000000000", 16)
	if ranges[0].Start.Cmp(wantStart) != 0 {
		t.Fatalf("unexpected start for chunk 1: got %s, want %s",
			ranges[0].Start.Text(16), wantStart.Text(16))
	}

	size := ranges[0].Size()
	expectedSize := new(big.Int).Sub(ranges[0].End, ranges[0].Start)
	expectedSize.Add(expectedSize, big.NewInt(1))
	if size.Cmp(expectedSize) != 0 {
		t.Fatalf("unexpected range size: got %s, want %s", size,
			expectedSize)
	}
}

func TestLoadRangesInvalid(t *testing.T) {
	dir := t.TempDir()

	cases := map[string]string{
		"missing-colon.txt":   "deadbeef\n",
		"bad-hex.txt":         "zzzz:ffff\n",
		"start-after-end.txt": "ff:00\n",
		"empty.txt":           "",
	}

	for name, contents := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
		if _, err := loadRanges(path); err == nil {
			t.Errorf("expected loadRanges(%s) to fail for contents %q",
				name, contents)
		}
	}
}

func TestLoadRangesMissingFile(t *testing.T) {
	if _, err := loadRanges("/nonexistent/path/ranges.txt"); err == nil {
		t.Fatal("expected an error for a missing ranges file")
	}
}

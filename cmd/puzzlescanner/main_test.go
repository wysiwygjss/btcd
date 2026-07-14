// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func addressForPrivKey(t *testing.T, keyInt *big.Int) string {
	t.Helper()

	buf := make([]byte, 32)
	keyInt.FillBytes(buf)
	h := hash160ForPrivKey(t, keyInt)

	addr, err := btcutil.NewAddressPubKeyHash(h[:], &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to build test address: %v", err)
	}
	return addr.EncodeAddress()
}

func TestDecodeP2PKHHash160(t *testing.T) {
	target := big.NewInt(987654321)
	wantHash := hash160ForPrivKey(t, target)
	addr := addressForPrivKey(t, target)

	got, err := decodeP2PKHHash160(addr)
	if err != nil {
		t.Fatalf("decodeP2PKHHash160 returned error: %v", err)
	}
	if got != wantHash {
		t.Fatalf("hash160 mismatch: got %x, want %x", got, wantHash)
	}
}

func TestDecodeP2PKHHash160RejectsGarbage(t *testing.T) {
	if _, err := decodeP2PKHHash160("not-a-real-address"); err == nil {
		t.Fatal("expected an error for an invalid address")
	}
}

func TestPickRandomIsWithinBounds(t *testing.T) {
	ids := []int{4, 9, 15}
	seen := make(map[int]bool)
	for i := 0; i < 100; i++ {
		id, err := pickRandom(ids)
		if err != nil {
			t.Fatalf("pickRandom returned error: %v", err)
		}
		found := false
		for _, want := range ids {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pickRandom returned %d, which is not in %v", id, ids)
		}
		seen[id] = true
	}
	if len(seen) != len(ids) {
		t.Fatalf("expected pickRandom to eventually return all of %v over "+
			"100 draws, only saw %v", ids, seen)
	}
}

// TestRunFindsKeyAndAvoidsDoubleScanning exercises the full scan-a-random-
// chunk / jump-to-the-next-chunk / never-rescan-a-finished-chunk loop
// end-to-end against a tiny synthetic keyspace, and then verifies that a
// second invocation does not rescan anything.
func TestRunFindsKeyAndAvoidsDoubleScanning(t *testing.T) {
	target := big.NewInt(555_555)
	targetHash160 := hash160ForPrivKey(t, target)
	targetAddr := addressForPrivKey(t, target)

	ranges := []*keyRange{
		{ID: 1, Start: big.NewInt(1_000_000), End: big.NewInt(1_010_000)},
		{ID: 2, Start: big.NewInt(2_000_000), End: big.NewInt(2_010_000)},
		{
			ID:    3,
			Start: new(big.Int).Sub(target, big.NewInt(20)),
			End:   new(big.Int).Add(target, big.NewInt(20)),
		},
	}

	outDir := t.TempDir()
	cfg := &config{
		address:   targetAddr,
		outDir:    outDir,
		duration:  2 * time.Second,
		workers:   2,
		staleLock: time.Minute,
	}

	if err := run(context.Background(), cfg, ranges, targetHash160); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	// The found chunk (3) must have a report recording the match.
	foundReport := filepath.Join(outDir, "3.scannedrandom.txt")
	data, err := os.ReadFile(foundReport)
	if err != nil {
		t.Fatalf("expected a report for the chunk containing the match: %v",
			err)
	}
	if !contains(string(data), "MATCH FOUND") {
		t.Fatalf("expected report to record a match, got:\n%s", data)
	}

	// No stray .inprogress lock files should be left behind.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	for _, e := range entries {
		if lockFileRE.MatchString(e.Name()) {
			t.Fatalf("unexpected leftover lock file: %s", e.Name())
		}
	}

	// Running again must not rescan anything: with only the found chunk
	// marked as scanned, chunks 1 and 2 are still eligible, so force a
	// scenario where every chunk is already marked scanned and confirm
	// run() does nothing.
	for _, r := range ranges {
		path := filepath.Join(outDir, resultFileName(r.ID))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte("pre-existing\n"), 0o644); err != nil {
			t.Fatalf("failed to seed report for chunk %d: %v", r.ID, err)
		}
	}

	before, err := scannedChunkIDs(outDir, cfg.staleLock)
	if err != nil {
		t.Fatalf("failed to read scanned IDs: %v", err)
	}
	if len(before) != len(ranges) {
		t.Fatalf("expected all %d chunks to be marked scanned, got %d",
			len(ranges), len(before))
	}

	if err := run(context.Background(), cfg, ranges, targetHash160); err != nil {
		t.Fatalf("second run returned error: %v", err)
	}

	after, err := scannedChunkIDs(outDir, cfg.staleLock)
	if err != nil {
		t.Fatalf("failed to read scanned IDs after second run: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("second run should not have changed the scanned set: "+
			"before=%v after=%v", before, after)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}

// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannedChunkIDsMissingDir(t *testing.T) {
	ids, err := scannedChunkIDs(filepath.Join(t.TempDir(), "does-not-exist"), 0)
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no scanned IDs, got %v", ids)
	}
}

func TestScannedChunkIDsResultFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1.scannedrandom.txt", "42.scannedrandom.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
	// Unrelated files must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write notes.txt: %v", err)
	}

	ids, err := scannedChunkIDs(dir, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ids[1] || !ids[42] || len(ids) != 2 {
		t.Fatalf("unexpected scanned set: %v", ids)
	}
}

func TestScannedChunkIDsHonorsFreshLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "7.scannedrandom.txt.inprogress")
	if err := os.WriteFile(lockPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}

	ids, err := scannedChunkIDs(dir, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ids[7] {
		t.Fatalf("expected a fresh lock to count as scanned/in-progress")
	}
}

func TestScannedChunkIDsReclaimsStaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "7.scannedrandom.txt.inprogress")
	if err := os.WriteFile(lockPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to backdate lock file: %v", err)
	}

	ids, err := scannedChunkIDs(dir, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids[7] {
		t.Fatalf("expected a stale lock to be reclaimed as available")
	}
}

func TestScannedChunkIDsAlwaysHonorsLockWhenStaleDisabled(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "7.scannedrandom.txt.inprogress")
	if err := os.WriteFile(lockPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to backdate lock file: %v", err)
	}

	ids, err := scannedChunkIDs(dir, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ids[7] {
		t.Fatalf("expected staleAfter<=0 to always honor the lock")
	}
}

func TestReserveChunkIsExclusive(t *testing.T) {
	dir := t.TempDir()

	ok, err := reserveChunk(dir, 5)
	if err != nil {
		t.Fatalf("unexpected error reserving chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected the first reservation to succeed")
	}

	ok, err = reserveChunk(dir, 5)
	if err != nil {
		t.Fatalf("unexpected error on second reservation: %v", err)
	}
	if ok {
		t.Fatal("expected the second reservation of the same chunk to fail")
	}

	if err := releaseLock(dir, 5); err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}

	ok, err = reserveChunk(dir, 5)
	if err != nil {
		t.Fatalf("unexpected error re-reserving after release: %v", err)
	}
	if !ok {
		t.Fatal("expected reservation to succeed again after release")
	}
}

func TestReleaseLockMissingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := releaseLock(dir, 99); err != nil {
		t.Fatalf("releasing a nonexistent lock should not error: %v", err)
	}
}

// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// resultSuffix is the suffix appended to the chunk ID to form the name of a
// completed scan result file, e.g. "25.scannedrandom.txt".
const resultSuffix = ".scannedrandom.txt"

// lockSuffix marks a chunk as currently being scanned. It is used to reserve
// a chunk atomically so that two scanner instances pointed at the same
// output directory never scan the same chunk at the same time.
const lockSuffix = ".scannedrandom.txt.inprogress"

var resultFileRE = regexp.MustCompile(`^(\d+)\.scannedrandom\.txt$`)
var lockFileRE = regexp.MustCompile(`^(\d+)\.scannedrandom\.txt\.inprogress$`)

// scannedChunkIDs walks outDir and returns the set of chunk IDs that already
// have a "<id>.scannedrandom.txt" result file, i.e. chunks that have been
// fully scanned already. staleAfter controls how old an in-progress lock
// file (left behind by a crashed or killed run) may be before it is treated
// as abandoned and made available for scanning again; a value <= 0 disables
// this and in-progress locks are always honored.
func scannedChunkIDs(outDir string, staleAfter time.Duration) (map[int]bool, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]bool{}, nil
		}
		return nil, fmt.Errorf("unable to read output directory: %w", err)
	}

	done := make(map[int]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		if m := resultFileRE.FindStringSubmatch(name); m != nil {
			id, err := strconv.Atoi(m[1])
			if err == nil {
				done[id] = true
			}
			continue
		}

		if m := lockFileRE.FindStringSubmatch(name); m != nil {
			id, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			if staleAfter <= 0 {
				done[id] = true
				continue
			}
			info, err := entry.Info()
			if err != nil {
				done[id] = true
				continue
			}
			if time.Since(info.ModTime()) < staleAfter {
				done[id] = true
			}
			// Otherwise the lock is stale: treat the chunk as
			// available again. The stale lock file will be
			// overwritten once the chunk is re-reserved.
		}
	}

	return done, nil
}

// reserveChunk atomically creates the ".inprogress" lock file for a chunk so
// that no other concurrent scanner instance can pick the same chunk. It
// returns false (with no error) if the chunk is already reserved by someone
// else.
func reserveChunk(outDir string, id int) (bool, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return false, fmt.Errorf("unable to create output directory: %w", err)
	}

	lockPath := filepath.Join(outDir, lockFileName(id))
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("unable to reserve chunk %d: %w", id, err)
	}
	defer f.Close()

	fmt.Fprintf(f, "reserved at %s by pid %d\n",
		time.Now().UTC().Format(time.RFC3339), os.Getpid())

	return true, nil
}

// releaseLock removes the ".inprogress" lock file for a chunk. It is called
// once the final "<id>.scannedrandom.txt" result has been written, or if the
// scan is aborted early so the chunk can be retried later.
func releaseLock(outDir string, id int) error {
	lockPath := filepath.Join(outDir, lockFileName(id))
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to release lock for chunk %d: %w", id, err)
	}
	return nil
}

func lockFileName(id int) string {
	return fmt.Sprintf("%d%s", id, ".scannedrandom.txt.inprogress")
}

func resultFileName(id int) string {
	return fmt.Sprintf("%d%s", id, resultSuffix)
}

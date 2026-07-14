// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// puzzlescanner randomly scans the chunks of a large private-key keyspace
// (such as the well known Bitcoin Puzzle #71 challenge, whose 10,000-chunk
// keyspace ships in testdata/puzzle71_10000.txt) looking for the private key
// behind a given P2PKH address.
//
// Every run of the tool:
//   - loads the full list of chunks (segments) from the ranges file;
//   - compares that list against the "<id>.scannedrandom.txt" result files
//     already present in the output directory so that a chunk already
//     scanned (by this run or an earlier one) is never scanned again;
//   - picks one of the remaining, not-yet-scanned chunks at random;
//   - randomly samples private keys from within that chunk for up to
//     -duration (default 30m), deriving each candidate's address and
//     comparing it against the target;
//   - writes the outcome to "<id>.scannedrandom.txt" (including the date
//     and time the scan started/ended), then randomly jumps to a new,
//     still-unscanned chunk and repeats.
//
// If a match is ever found, the tool prints and saves the private key and
// stops immediately.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// puzzle71Address is the target for the well known, still-unsolved Bitcoin
// Puzzle #71 challenge. It is used as the default target address.
const puzzle71Address = "1PWo3JeB9jrGwfHDNpdGK54CRas7fsVzXU"

func mainNetParams() *chaincfg.Params {
	return &chaincfg.MainNetParams
}

type config struct {
	rangesPath string
	address    string
	outDir     string
	duration   time.Duration
	workers    int
	once       bool
	staleLock  time.Duration
}

func parseFlags() *config {
	cfg := &config{}

	flag.StringVar(&cfg.rangesPath, "ranges",
		"cmd/puzzlescanner/testdata/puzzle71_10000.txt",
		"path to the chunk list file (lines of \"<start-hex>:<end-hex>\")")
	flag.StringVar(&cfg.address, "address", puzzle71Address,
		"target P2PKH address to search for")
	flag.StringVar(&cfg.outDir, "outdir", "scannedchunks",
		"directory to store <id>.scannedrandom.txt result files in")
	flag.DurationVar(&cfg.duration, "duration", 30*time.Minute,
		"how long to randomly scan each chunk before jumping to the next one")
	flag.IntVar(&cfg.workers, "workers", runtime.NumCPU(),
		"number of concurrent workers used while scanning a chunk")
	flag.BoolVar(&cfg.once, "once", false,
		"scan a single random chunk and then exit, instead of looping forever")
	staleMultiplier := flag.Float64("stale-lock-multiplier", 2.0,
		"treat an .inprogress lock left by a crashed run as abandoned once "+
			"it is older than duration * this multiplier")
	flag.Parse()

	cfg.staleLock = time.Duration(float64(cfg.duration) * *staleMultiplier)

	return cfg
}

func main() {
	cfg := parseFlags()

	if cfg.workers < 1 {
		cfg.workers = 1
	}

	ranges, err := loadRanges(cfg.rangesPath)
	if err != nil {
		log.Fatalf("failed to load ranges: %v", err)
	}
	log.Printf("loaded %d chunks from %s", len(ranges), cfg.rangesPath)

	targetHash160, err := decodeP2PKHHash160(cfg.address)
	if err != nil {
		log.Fatalf("failed to decode target address %q: %v", cfg.address, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, cfg, ranges, targetHash160); err != nil {
		log.Fatal(err)
	}
}

// run drives the randomized scan-a-chunk / jump-to-a-new-chunk loop until
// every chunk has been scanned, a match is found, -once is set, or ctx is
// canceled.
func run(
	ctx context.Context,
	cfg *config,
	ranges []*keyRange,
	targetHash160 [20]byte,
) error {

	byID := make(map[int]*keyRange, len(ranges))
	for _, r := range ranges {
		byID[r.ID] = r
	}

	for {
		if ctx.Err() != nil {
			log.Printf("stopping: %v", ctx.Err())
			return nil
		}

		scanned, err := scannedChunkIDs(cfg.outDir, cfg.staleLock)
		if err != nil {
			return fmt.Errorf("failed to inspect %s: %w", cfg.outDir, err)
		}

		var remaining []int
		for id := range byID {
			if !scanned[id] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) == 0 {
			log.Printf("all %d chunks have already been scanned; nothing "+
				"left to do", len(ranges))
			return nil
		}

		id, err := pickRandom(remaining)
		if err != nil {
			return fmt.Errorf("failed to pick a random chunk: %w", err)
		}

		reserved, err := reserveChunk(cfg.outDir, id)
		if err != nil {
			return fmt.Errorf("failed to reserve chunk %d: %w", id, err)
		}
		if !reserved {
			// Lost a race with another concurrent scanner instance
			// pointed at the same output directory; try again with
			// a fresh random pick.
			continue
		}

		chunk := byID[id]
		log.Printf("scanning chunk %d/%d  [%s, %s]  for up to %s",
			id, len(ranges), chunk.Start.Text(16), chunk.End.Text(16),
			cfg.duration)

		res, err := scanChunk(ctx, chunk, targetHash160, cfg.address,
			cfg.workers, cfg.duration, progressLogger(id))
		if err != nil {
			releaseLock(cfg.outDir, id)
			return fmt.Errorf("failed to scan chunk %d: %w", id, err)
		}

		path, err := writeReport(cfg.outDir, cfg.address, res)
		if err != nil {
			releaseLock(cfg.outDir, id)
			return fmt.Errorf("failed to write report for chunk %d: %w",
				id, err)
		}
		if err := releaseLock(cfg.outDir, id); err != nil {
			log.Printf("warning: %v", err)
		}

		log.Printf("chunk %d done: tried %d keys in %s (%.0f keys/sec); "+
			"saved %s", id, res.KeysTried, res.Duration().Round(time.Second),
			res.KeysPerSecond(), path)

		if res.Found {
			log.Printf("*** MATCH FOUND in chunk %d ***", id)
			log.Printf("private key (hex): %064x", res.FoundKey)
			log.Printf("private key (WIF): %s", res.FoundWIF)
			log.Printf("address:            %s", res.FoundAddr)
			return nil
		}

		if cfg.once {
			return nil
		}
	}
}

func progressLogger(chunkID int) func(int64, time.Duration) {
	return func(keysTried int64, elapsed time.Duration) {
		rate := float64(0)
		if secs := elapsed.Seconds(); secs > 0 {
			rate = float64(keysTried) / secs
		}
		log.Printf("chunk %d: %d keys tried so far (%.0f keys/sec)",
			chunkID, keysTried, rate)
	}
}

// decodeP2PKHHash160 decodes a base58check P2PKH address and returns the
// 20-byte hash160 it commits to.
func decodeP2PKHHash160(address string) ([20]byte, error) {
	var out [20]byte

	addr, err := btcutil.DecodeAddress(address, mainNetParams())
	if err != nil {
		return out, err
	}
	pkHashAddr, ok := addr.(*btcutil.AddressPubKeyHash)
	if !ok {
		return out, fmt.Errorf("address %s is not a P2PKH address", address)
	}
	copy(out[:], pkHashAddr.Hash160()[:])

	return out, nil
}

// pickRandom returns a uniformly random element of ids using a
// cryptographically secure source of randomness.
func pickRandom(ids []int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(ids))))
	if err != nil {
		return 0, err
	}
	return ids[n.Int64()], nil
}

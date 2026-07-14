// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

// scanResult summarizes what happened while a chunk was scanned.
type scanResult struct {
	Chunk     *keyRange
	StartTime time.Time
	EndTime   time.Time
	KeysTried int64
	Found     bool
	FoundKey  *big.Int
	FoundWIF  string
	FoundAddr string
}

// Duration is a convenience helper for reporting.
func (r *scanResult) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// KeysPerSecond is a convenience helper for reporting.
func (r *scanResult) KeysPerSecond() float64 {
	secs := r.Duration().Seconds()
	if secs <= 0 {
		return 0
	}
	return float64(r.KeysTried) / secs
}

// scanChunk randomly samples private keys from within chunk's [Start, End]
// range using `workers` concurrent goroutines, hashing each candidate public
// key and comparing it against targetHash160. It keeps sampling until
// maxDuration elapses, a match is found, or ctx is canceled, whichever comes
// first. progress, if non-nil, is invoked periodically with a running
// snapshot of the keys tried so far so callers can print progress updates.
func scanChunk(
	ctx context.Context,
	chunk *keyRange,
	targetHash160 [20]byte,
	targetAddr string,
	workers int,
	maxDuration time.Duration,
	progress func(keysTried int64, elapsed time.Duration),
) (*scanResult, error) {

	if workers < 1 {
		workers = 1
	}

	size := chunk.Size()
	if size.Sign() <= 0 {
		return nil, fmt.Errorf("chunk %d has an empty range", chunk.ID)
	}

	scanCtx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	result := &scanResult{
		Chunk:     chunk,
		StartTime: time.Now().UTC(),
	}

	var (
		keysTried int64
		foundOnce sync.Once
		wg        sync.WaitGroup
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			buf := make([]byte, 32)
			for {
				select {
				case <-scanCtx.Done():
					return
				default:
				}

				key, err := randomKeyInRange(chunk.Start, size)
				if err != nil {
					// crypto/rand failures are unrecoverable; stop
					// this worker's sampling loop.
					return
				}
				atomic.AddInt64(&keysTried, 1)

				key.FillBytes(buf)
				matched := checkCandidate(buf, targetHash160)
				if matched {
					foundOnce.Do(func() {
						result.Found = true
						result.FoundKey = new(big.Int).Set(key)
						cancel()
					})
					return
				}
			}
		}()
	}

	// Report progress until every worker has exited.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
progressLoop:
	for {
		select {
		case <-done:
			break progressLoop
		case <-ticker.C:
			if progress != nil {
				progress(atomic.LoadInt64(&keysTried),
					time.Since(result.StartTime))
			}
		}
	}

	result.EndTime = time.Now().UTC()
	result.KeysTried = atomic.LoadInt64(&keysTried)

	if result.Found {
		priv, _ := btcec.PrivKeyFromBytes(padTo32(result.FoundKey))
		wif, err := btcutil.NewWIF(priv, mainNetParams(), true)
		if err == nil {
			result.FoundWIF = wif.String()
		}
		result.FoundAddr = targetAddr
	}

	return result, nil
}

// randomKeyInRange returns a cryptographically random big.Int uniformly
// distributed in [start, start+size).
func randomKeyInRange(start *big.Int, size *big.Int) (*big.Int, error) {
	offset, err := rand.Int(rand.Reader, size)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Add(start, offset), nil
}

// checkCandidate derives the compressed and uncompressed public keys for the
// 32-byte big-endian private key in keyBytes and reports whether either
// hash160 matches targetHash160.
func checkCandidate(keyBytes []byte, targetHash160 [20]byte) bool {
	_, pub := btcec.PrivKeyFromBytes(keyBytes)

	compressed := btcutil.Hash160(pub.SerializeCompressed())
	if hash160Equal(compressed, targetHash160) {
		return true
	}

	uncompressed := btcutil.Hash160(pub.SerializeUncompressed())
	return hash160Equal(uncompressed, targetHash160)
}

func hash160Equal(h []byte, target [20]byte) bool {
	if len(h) != 20 {
		return false
	}
	for i := 0; i < 20; i++ {
		if h[i] != target[i] {
			return false
		}
	}
	return true
}

func padTo32(n *big.Int) []byte {
	buf := make([]byte, 32)
	n.FillBytes(buf)
	return buf
}

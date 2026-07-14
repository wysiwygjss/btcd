// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

// hash160ForPrivKey is a small test helper that derives the compressed
// pubkey hash160 for a given private key integer.
func hash160ForPrivKey(t *testing.T, keyInt *big.Int) [20]byte {
	t.Helper()

	buf := make([]byte, 32)
	keyInt.FillBytes(buf)
	_, pub := btcec.PrivKeyFromBytes(buf)
	h := btcutil.Hash160(pub.SerializeCompressed())

	var out [20]byte
	copy(out[:], h)
	return out
}

func TestScanChunkFindsKeyInSmallRange(t *testing.T) {
	target := big.NewInt(123456789)
	targetHash160 := hash160ForPrivKey(t, target)

	chunk := &keyRange{
		ID:    1,
		Start: new(big.Int).Sub(target, big.NewInt(50)),
		End:   new(big.Int).Add(target, big.NewInt(50)),
	}

	res, err := scanChunk(context.Background(), chunk, targetHash160,
		"test-address", 4, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("scanChunk returned error: %v", err)
	}
	if !res.Found {
		t.Fatalf("expected the tiny range to be exhaustively covered and "+
			"the key found; tried %d keys", res.KeysTried)
	}
	if res.FoundKey.Cmp(target) != 0 {
		t.Fatalf("found wrong key: got %s, want %s", res.FoundKey,
			target)
	}
	if res.FoundWIF == "" {
		t.Fatal("expected a WIF to be populated for a found key")
	}
}

func TestScanChunkRespectsDeadlineWhenNotFound(t *testing.T) {
	// A target that cannot possibly be in the range.
	var targetHash160 [20]byte
	copy(targetHash160[:], []byte("deadbeefdeadbeefdead"))

	chunk := &keyRange{
		ID:    2,
		Start: big.NewInt(1_000_000),
		End:   big.NewInt(2_000_000),
	}

	start := time.Now()
	res, err := scanChunk(context.Background(), chunk, targetHash160,
		"test-address", 2, 300*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("scanChunk returned error: %v", err)
	}
	elapsed := time.Since(start)

	if res.Found {
		t.Fatal("did not expect to find a match for an impossible target")
	}
	if res.KeysTried == 0 {
		t.Fatal("expected at least some keys to have been tried")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("scanChunk took too long to respect its deadline: %s",
			elapsed)
	}
}

func TestScanChunkRespectsContextCancellation(t *testing.T) {
	var targetHash160 [20]byte
	copy(targetHash160[:], []byte("deadbeefdeadbeefdead"))

	chunk := &keyRange{
		ID:    3,
		Start: big.NewInt(1_000_000),
		End:   big.NewInt(2_000_000),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := scanChunk(ctx, chunk, targetHash160, "test-address", 2,
		time.Minute, nil)
	if err != nil {
		t.Fatalf("scanChunk returned error: %v", err)
	}
	if res.Found {
		t.Fatal("did not expect a match")
	}
}

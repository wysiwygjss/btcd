// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// finding describes a successfully recovered private key, along with the
// two records whose shared R value led to its recovery.
type finding struct {
	pubKeyHex  string
	pubKey     *btcec.PublicKey
	rHex       string
	rec1, rec2 *record
	privKey    *btcec.PrivateKey
}

// detect groups records by public key and R value and attempts to recover
// the private key for every group that contains two or more records signed
// with the same public key that share an R value but differ in S (i.e. a
// genuine nonce-reuse pair, as opposed to two copies of the exact same
// signature).
//
// It returns one finding per public key for which a private key was
// successfully recovered and independently verified against that public
// key; a given signer contributing multiple colliding R groups will only
// ever need one to leak its key, but detect does not stop early so that the
// report can show every distinct colliding pair.
func detect(records []*record) ([]*finding, error) {
	// Group by signer, then by R value, so that a single scan of the
	// input handles signers with many records and/or many distinct
	// colliding R values.
	bySigner := make(map[string][]*record)
	for _, rec := range records {
		bySigner[rec.pubKeyGroupKey()] = append(bySigner[rec.pubKeyGroupKey()], rec)
	}

	var findings []*finding
	for _, signerRecords := range bySigner {
		byR := make(map[string][]*record)
		for _, rec := range signerRecords {
			byR[rec.rHex()] = append(byR[rec.rHex()], rec)
		}

		for rHex, group := range byR {
			if len(group) < 2 {
				continue
			}

			f, err := recoverFromGroup(rHex, group)
			if err != nil {
				return nil, err
			}
			if f != nil {
				findings = append(findings, f)
			}
		}
	}

	return findings, nil
}

// recoverFromGroup attempts nonce-reuse recovery across every distinct pair
// of records in group (all of which share the same signer and R value),
// returning the first successfully verified recovery, or nil if none of the
// pairs yield one (e.g. because the "duplicates" are all exact copies of a
// single signature and therefore carry no information about the nonce).
func recoverFromGroup(rHex string, group []*record) (*finding, error) {
	for i := 0; i < len(group); i++ {
		for j := i + 1; j < len(group); j++ {
			rec1, rec2 := group[i], group[j]
			if rec1.s == rec2.s {
				// Exact duplicate signature; no new information.
				continue
			}

			privKey, err := ecdsa.RecoverPrivateKeyFromDuplicateR(
				rec1.r[:], rec1.s[:], rec2.s[:], rec1.z, rec2.z,
			)
			if errors.Is(err, ecdsa.ErrNonceReuseNotDetected) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("unexpected error recovering "+
					"private key for records %q/%q: %w", rec1.id,
					rec2.id, err)
			}

			if !privKey.PubKey().IsEqual(rec1.pubKey) {
				// The self-check inside RecoverPrivateKeyFromDuplicateR
				// passed, but the recovered key does not actually
				// correspond to the claimed signer. This should be
				// essentially impossible in practice; treat it as
				// "not a real finding" rather than reporting a
				// misleading result.
				continue
			}

			return &finding{
				pubKeyHex: rec1.pubKeyHex,
				pubKey:    rec1.pubKey,
				rHex:      rHex,
				rec1:      rec1,
				rec2:      rec2,
				privKey:   privKey,
			}, nil
		}
	}

	return nil, nil
}

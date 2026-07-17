// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
)

// signWithNonce manually produces an ECDSA signature over hash using privKey
// and the supplied (non-secret, attacker-controlled for test purposes) nonce
// k, following the standard signing equation s = k^-1 * (e + d*r) mod N. It
// exists solely to construct nonce-reuse test fixtures, since the package's
// public Sign function always derives its nonce deterministically via
// RFC6979 and therefore cannot be coerced into reusing a nonce across two
// different messages.
func signWithNonce(t *testing.T, privKey *btcec.PrivateKey, k *btcec.ModNScalar,
	hash []byte) *Signature {

	t.Helper()

	var kG btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(k, &kG)
	kG.ToAffine()

	var r btcec.ModNScalar
	xBytes := kG.X.Bytes()
	if overflow := r.SetByteSlice(xBytes[:]); overflow || r.IsZero() {
		t.Fatalf("bad test nonce: r invalid")
	}

	var e btcec.ModNScalar
	e.SetByteSlice(hash)

	d := &privKey.Key
	var dr, sum, kInv, s btcec.ModNScalar
	dr.Mul2(d, &r)
	sum.Add2(&e, &dr)
	kInv.InverseValNonConst(k)
	s.Mul2(&sum, &kInv)
	if s.IsZero() {
		t.Fatalf("bad test nonce: s is zero")
	}

	return NewSignature(&r, &s)
}

// hashOf is a small helper that returns the sha256 digest of msg, suitable
// for use as an ECDSA message hash in tests.
func hashOf(msg string) []byte {
	h := sha256.Sum256([]byte(msg))
	return h[:]
}

// rawSign computes the raw (r, s) signature components for hash using
// privKey and nonce k, following the standard ECDSA signing equation s =
// k^-1 * (e + d*r) mod N, WITHOUT applying the low-S (BIP0062) canonicalization
// that Signature.Serialize always performs. This lets tests precisely control
// whether the resulting s value would or would not require the low-S flip,
// which is needed to exercise both sign conventions handled by
// RecoverPrivateKeyFromDuplicateR.
func rawSign(t *testing.T, privKey *btcec.PrivateKey, k *btcec.ModNScalar,
	hash []byte) (r, s [32]byte) {

	t.Helper()

	var kG btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(k, &kG)
	kG.ToAffine()

	var rScalar btcec.ModNScalar
	xBytes := kG.X.Bytes()
	if overflow := rScalar.SetByteSlice(xBytes[:]); overflow || rScalar.IsZero() {
		t.Fatalf("bad test nonce: r invalid")
	}

	var e btcec.ModNScalar
	e.SetByteSlice(hash)

	d := &privKey.Key
	var dr, sum, kInv, sScalar btcec.ModNScalar
	dr.Mul2(d, &rScalar)
	sum.Add2(&e, &dr)
	kInv.InverseValNonConst(k)
	sScalar.Mul2(&sum, &kInv)
	if sScalar.IsZero() {
		t.Fatalf("bad test nonce: s is zero")
	}

	rScalar.PutBytes(&r)
	sScalar.PutBytes(&s)

	return r, s
}

// TestRecoverPrivateKeyFromDuplicateR verifies that reusing the same nonce to
// sign two different messages with the same private key allows the private
// key to be fully recovered from nothing but the two resulting signatures
// and the two message hashes.
func TestRecoverPrivateKeyFromDuplicateR(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	var k btcec.ModNScalar
	k.SetInt(424242)

	hash1 := hashOf("first message, signed with a reused nonce")
	hash2 := hashOf("a completely different second message")

	sig1 := signWithNonce(t, privKey, &k, hash1)
	sig2 := signWithNonce(t, privKey, &k, hash2)

	// Sanity check: both signatures must independently verify against the
	// signer's public key.
	pubKey := privKey.PubKey()
	if !sig1.Verify(hash1, pubKey) {
		t.Fatalf("sig1 failed to verify")
	}
	if !sig2.Verify(hash2, pubKey) {
		t.Fatalf("sig2 failed to verify")
	}

	r1, s1 := ComponentsFromSignature(sig1)
	r2, s2 := ComponentsFromSignature(sig2)
	if r1 != r2 {
		t.Fatalf("expected both signatures to share r, got %x and %x", r1, r2)
	}

	recovered, err := RecoverPrivateKeyFromDuplicateR(
		r1[:], s1[:], s2[:], hash1, hash2,
	)
	if err != nil {
		t.Fatalf("unexpected error recovering private key: %v", err)
	}

	if !recovered.Key.Equals(&privKey.Key) {
		t.Fatalf("recovered private key does not match original\n"+
			"got:  %x\nwant: %x", recovered.Serialize(),
			privKey.Serialize())
	}

	if !recovered.PubKey().IsEqual(pubKey) {
		t.Fatalf("recovered public key does not match original")
	}
}

// TestRecoverPrivateKeyFromSignatures exercises the Signature-based
// convenience wrapper around RecoverPrivateKeyFromDuplicateR, including the
// case where the two signatures do not actually share an r value.
func TestRecoverPrivateKeyFromSignatures(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	var k btcec.ModNScalar
	k.SetInt(1337)

	hash1 := hashOf("reused-nonce message one")
	hash2 := hashOf("reused-nonce message two")

	sig1 := signWithNonce(t, privKey, &k, hash1)
	sig2 := signWithNonce(t, privKey, &k, hash2)

	recovered, err := RecoverPrivateKeyFromSignatures(sig1, hash1, sig2, hash2)
	if err != nil {
		t.Fatalf("unexpected error recovering private key: %v", err)
	}
	if !recovered.Key.Equals(&privKey.Key) {
		t.Fatalf("recovered private key does not match original")
	}

	// A signature produced with a fresh, non-reused nonce should not share
	// an r value with sig1, and recovery should fail cleanly.
	freshSig := Sign(privKey, hashOf("a message signed with a fresh nonce"))
	_, err = RecoverPrivateKeyFromSignatures(
		sig1, hash1, freshSig, hashOf("a message signed with a fresh nonce"),
	)
	if !errors.Is(err, ErrDifferentR) {
		t.Fatalf("expected ErrDifferentR, got %v", err)
	}
}

// TestRecoverPrivateKeyFromDuplicateRErrors verifies the various error
// conditions RecoverPrivateKeyFromDuplicateR is expected to detect.
func TestRecoverPrivateKeyFromDuplicateRErrors(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	var k btcec.ModNScalar
	k.SetInt(9001)

	hash1 := hashOf("message one")
	hash2 := hashOf("message two")

	sig1 := signWithNonce(t, privKey, &k, hash1)
	sig2 := signWithNonce(t, privKey, &k, hash2)

	r1, s1 := ComponentsFromSignature(sig1)
	_, s2 := ComponentsFromSignature(sig2)

	zero := make([]byte, 32)

	tests := []struct {
		name      string
		r, s1, s2 []byte
		hash1     []byte
		hash2     []byte
		wantErr   error
	}{
		{
			name:    "zero r",
			r:       zero,
			s1:      s1[:],
			s2:      s2[:],
			hash1:   hash1,
			hash2:   hash2,
			wantErr: ErrInvalidR,
		},
		{
			name:    "zero s1",
			r:       r1[:],
			s1:      zero,
			s2:      s2[:],
			hash1:   hash1,
			hash2:   hash2,
			wantErr: ErrInvalidS,
		},
		{
			name:    "zero s2",
			r:       r1[:],
			s1:      s1[:],
			s2:      zero,
			hash1:   hash1,
			hash2:   hash2,
			wantErr: ErrInvalidS,
		},
		{
			name:    "identical signatures",
			r:       r1[:],
			s1:      s1[:],
			s2:      s1[:],
			hash1:   hash1,
			hash2:   hash1,
			wantErr: ErrNonceReuseNotDetected,
		},
		{
			name: "self-check failure on unrelated inputs",
			r:    r1[:],
			s1:   s1[:],
			// A different, unrelated valid-looking s along with a
			// hash that was never actually signed with this r/k
			// should fail the internal k*G self-check for both
			// sign conventions.
			s2:      s2[:],
			hash1:   hash1,
			hash2:   hashOf("an unrelated message never signed with this nonce"),
			wantErr: ErrNonceReuseNotDetected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := RecoverPrivateKeyFromDuplicateR(
				test.r, test.s1, test.s2, test.hash1, test.hash2,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("expected error %v, got %v", test.wantErr, err)
			}
		})
	}
}

// TestRecoverPrivateKeyFromDuplicateRSignConventions specifically exercises
// both of the sign conventions RecoverPrivateKeyFromDuplicateR must handle:
// the "same sign" case where both raw s values retain the sign implied by
// the actual shared nonce, and the "opposite sign" case that arises when
// low-S (BIP0062) canonicalization is applied independently to each
// signature and ends up flipping exactly one of them.
func TestRecoverPrivateKeyFromDuplicateRSignConventions(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	var k btcec.ModNScalar
	k.SetInt(777777)

	hash1 := hashOf("sign convention message one")
	hash2 := hashOf("sign convention message two")

	r, s1 := rawSign(t, privKey, &k, hash1)
	_, s2 := rawSign(t, privKey, &k, hash2)

	var s2Scalar, negS2Scalar btcec.ModNScalar
	s2Scalar.SetByteSlice(s2[:])
	negS2Scalar.NegateVal(&s2Scalar)
	var negS2 [32]byte
	negS2Scalar.PutBytes(&negS2)

	testCases := []struct {
		name string
		s2   []byte
	}{
		{name: "same sign convention", s2: s2[:]},
		{name: "opposite sign convention", s2: negS2[:]},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recovered, err := RecoverPrivateKeyFromDuplicateR(
				r[:], s1[:], tc.s2, hash1, hash2,
			)
			if err != nil {
				t.Fatalf("unexpected error recovering private "+
					"key: %v", err)
			}
			if !recovered.Key.Equals(&privKey.Key) {
				t.Fatalf("recovered private key does not "+
					"match original\ngot:  %x\nwant: %x",
					recovered.Serialize(), privKey.Serialize())
			}
		})
	}
}

// TestComponentsFromSignatureRoundTrip verifies that the R and S values
// extracted via ComponentsFromSignature match the values used to construct
// the signature in the first place.
func TestComponentsFromSignatureRoundTrip(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	sig := Sign(privKey, hashOf("round trip message"))

	r, s := ComponentsFromSignature(sig)

	var rScalar, sScalar btcec.ModNScalar
	rScalar.SetByteSlice(r[:])
	sScalar.SetByteSlice(s[:])
	roundTripSig := NewSignature(&rScalar, &sScalar)
	if !bytes.Equal(sig.Serialize(), roundTripSig.Serialize()) {
		t.Fatalf("round-tripped signature does not match original")
	}
}

// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
)

// Errors returned when attempting to recover a private key from two
// signatures that are believed to share the same nonce (and therefore the
// same R value).
var (
	// ErrInvalidR is returned when the provided R value is not a valid
	// signature R component (i.e. it is zero or greater than or equal to
	// the group order).
	ErrInvalidR = errors.New("nonce reuse: r is not a valid signature R value")

	// ErrInvalidS is returned when one of the provided S values is not a
	// valid signature S component (i.e. it is zero or greater than or
	// equal to the group order).
	ErrInvalidS = errors.New("nonce reuse: s is not a valid signature S value")

	// ErrDifferentR is returned by RecoverPrivateKeyFromSignatures when
	// the two provided signatures do not share the same R value and
	// therefore do not exhibit the nonce reuse condition this package
	// looks for.
	ErrDifferentR = errors.New("nonce reuse: signatures do not share " +
		"the same r value")

	// ErrNonceReuseNotDetected is returned by
	// RecoverPrivateKeyFromDuplicateR when it is unable to recover a
	// private key that is self-consistent with the supplied R, S, and
	// hash values. In practice this means the two signatures, despite
	// sharing an R value, do not actually appear to have been produced
	// using the same nonce and private key (for example, the R value
	// collision happened by pure chance, or the caller paired up
	// unrelated signatures/hashes by mistake).
	ErrNonceReuseNotDetected = errors.New("nonce reuse: could not " +
		"recover a private key consistent with the given signatures; " +
		"they do not appear to share a nonce")
)

// recoverCandidate attempts one of the two possible sign conventions for the
// duplicate-R nonce reuse attack (see RecoverPrivateKeyFromDuplicateR for
// details on why two conventions are needed) and, if it produces a
// self-consistent private key, returns it.
//
// sDenom is either (s1 - s2) or (s1 + s2) mod N depending on which of the two
// conventions is being attempted.
func recoverCandidate(r, s1, e1, sDenom, eDiff *btcec.ModNScalar) (*btcec.PrivateKey, bool) {
	if sDenom.IsZero() {
		return nil, false
	}

	// k = eDiff / sDenom mod N.
	var sDenomInv, k btcec.ModNScalar
	sDenomInv.InverseValNonConst(sDenom)
	k.Mul2(eDiff, &sDenomInv)
	if k.IsZero() {
		return nil, false
	}

	// d = (s1*k - e1) / r mod N.
	var negE1, s1k, numerator, rInv, d btcec.ModNScalar
	negE1.NegateVal(e1)
	s1k.Mul2(s1, &k)
	numerator.Add2(&s1k, &negE1)
	rInv.InverseValNonConst(r)
	d.Mul2(&numerator, &rInv)
	if d.IsZero() {
		return nil, false
	}

	// Self-check: recompute R' = (k*G).x mod N using the recovered nonce
	// and confirm it matches the R value the two signatures were
	// observed to share. This is what actually determines whether this
	// candidate (as opposed to the other sign convention) is the correct
	// one, and it also guards against the (astronomically unlikely) case
	// of an R collision that isn't the result of genuine nonce reuse.
	var kG btcec.JacobianPoint
	btcec.ScalarBaseMultNonConst(&k, &kG)
	kG.ToAffine()
	var rCheck btcec.ModNScalar
	xBytes := kG.X.Bytes()
	rCheck.SetByteSlice(xBytes[:])
	if !rCheck.Equals(r) {
		return nil, false
	}

	return btcec.PrivKeyFromScalar(&d), true
}

// RecoverPrivateKeyFromDuplicateR recovers the ECDSA private key used to
// produce two signatures that were generated using the same secret nonce,
// and thus share the same signature R value, over two different message
// hashes.
//
// Nonce reuse is a well known catastrophic failure of the ECDSA signature
// scheme. Given signature (r, s1) over a message with hash e1, and signature
// (r, s2) over a different message with hash e2, both produced with the same
// private key d and the same secret nonce k, the following identities hold
// (per the standard ECDSA signing equation s = k^-1 * (e + d*r) mod N):
//
//	s1 = k^-1 * (e1 + d*r) mod N
//	s2 = k^-1 * (e2 + d*r) mod N
//
// Subtracting the two equations eliminates d*r and isolates k:
//
//	k = (e1 - e2) / (s1 - s2) mod N
//
// Once k is known, the private key falls out directly from either original
// equation:
//
//	d = (s1*k - e1) / r mod N
//
// There is one important real-world wrinkle: this package (like most ECDSA
// implementations, following BIP0062) always produces "low-S" signatures,
// negating s whenever it would otherwise exceed N/2. Since both s and -s mod
// N are valid signatures for a given (message, key, nonce) -- they simply
// correspond to nonces k and -k respectively, which produce the same R
// because negating a point does not change its X coordinate -- signatures
// collected from the wild (e.g. from a blockchain) may have independently
// had this normalization applied to each of them. That means the two
// observed S values might correspond to the same "sign" of the shared nonce
// or to opposite signs, and only one of:
//
//	k = (e1 - e2) / (s1 - s2) mod N
//	k = (e1 - e2) / (s1 + s2) mod N
//
// will actually be consistent with the observed R value. This function tries
// both and returns whichever produces a private key that is self-consistent
// (verified by recomputing k*G and checking that its X coordinate reduces to
// r), so callers do not need to worry about which normalization convention
// was used to produce the input signatures.
//
// rBytes, s1Bytes, and s2Bytes are the big-endian encoded signature R and S
// components (r is shared by both signatures, s1 and s2 are not), and
// hash1/hash2 are the message digests that were signed. All arguments follow
// the same encoding and semantics as the hash parameter accepted by Sign and
// Verify -- namely, they are interpreted as 256-bit big-endian integers and
// implicitly reduced modulo the group order.
//
// A non-nil private key is returned if and only if the internal
// self-consistency check described above passes. Callers that additionally
// know the public key associated with the signatures should further confirm
// the result against that public key, since the self-check alone cannot
// distinguish the correct private key from an unrelated one in the
// vanishingly unlikely event that two different keys' nonces happen to
// collide on the same R value.
func RecoverPrivateKeyFromDuplicateR(rBytes, s1Bytes, s2Bytes, hash1,
	hash2 []byte) (*btcec.PrivateKey, error) {

	var r, s1, s2 btcec.ModNScalar
	if overflow := r.SetByteSlice(rBytes); overflow || r.IsZero() {
		return nil, ErrInvalidR
	}
	if overflow := s1.SetByteSlice(s1Bytes); overflow || s1.IsZero() {
		return nil, ErrInvalidS
	}
	if overflow := s2.SetByteSlice(s2Bytes); overflow || s2.IsZero() {
		return nil, ErrInvalidS
	}

	var e1, e2 btcec.ModNScalar
	e1.SetByteSlice(hash1)
	e2.SetByteSlice(hash2)

	var negE2, eDiff btcec.ModNScalar
	negE2.NegateVal(&e2)
	eDiff.Add2(&e1, &negE2)

	// Case A: the two signatures use the same relative sign convention
	// for their nonce, so sDenom = s1 - s2.
	var negS2, sDiff btcec.ModNScalar
	negS2.NegateVal(&s2)
	sDiff.Add2(&s1, &negS2)
	if privKey, ok := recoverCandidate(&r, &s1, &e1, &sDiff, &eDiff); ok {
		return privKey, nil
	}

	// Case B: the two signatures were canonicalized independently and
	// ended up using opposite sign conventions for their (shared, up to
	// sign) nonce, so sDenom = s1 + s2.
	var sSum btcec.ModNScalar
	sSum.Add2(&s1, &s2)
	if privKey, ok := recoverCandidate(&r, &s1, &e1, &sSum, &eDiff); ok {
		return privKey, nil
	}

	return nil, ErrNonceReuseNotDetected
}

// ComponentsFromSignature extracts the raw R and S values that make up sig,
// each encoded as a 32-byte big-endian scalar. Signature intentionally does
// not expose its internal R and S fields, so this helper exists for callers
// that need direct access to them -- for example, to group or index
// signatures by their R value in order to detect nonce reuse, or to pass
// them to RecoverPrivateKeyFromDuplicateR.
func ComponentsFromSignature(sig *Signature) (r, s [32]byte) {
	// Signature.Serialize always produces a valid low-S DER encoding, so
	// parsing it back can never fail in practice.
	rs, ss, err := parseSigRS(sig.Serialize(), true)
	if err != nil {
		panic(fmt.Sprintf("unexpected error parsing serialized "+
			"signature: %v", err))
	}

	rs.PutBytes(&r)
	ss.PutBytes(&s)

	return r, s
}

// RecoverPrivateKeyFromSignatures recovers the ECDSA private key used to
// produce sig1 and sig2, given the message hashes hash1 and hash2 that they
// each sign over, under the assumption that both signatures were produced
// using the same (or negated, see RecoverPrivateKeyFromDuplicateR) nonce,
// equivalently that they share the same R value.
//
// It is a thin convenience wrapper around RecoverPrivateKeyFromDuplicateR
// for callers that already have parsed Signature values rather than the raw
// R/S byte encodings; see that function for details of the underlying
// recovery algorithm. ErrDifferentR is returned immediately, without
// attempting any recovery, if the two signatures do not in fact share an R
// value.
func RecoverPrivateKeyFromSignatures(sig1 *Signature, hash1 []byte,
	sig2 *Signature, hash2 []byte) (*btcec.PrivateKey, error) {

	r1, s1 := ComponentsFromSignature(sig1)
	r2, s2 := ComponentsFromSignature(sig2)
	if r1 != r2 {
		return nil, ErrDifferentR
	}

	return RecoverPrivateKeyFromDuplicateR(r1[:], s1[:], s2[:], hash1, hash2)
}

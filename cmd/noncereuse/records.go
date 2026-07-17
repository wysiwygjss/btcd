// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// record is a single observed ECDSA signature, along with the public key
// that produced it and the message hash it signs over. Records are the unit
// that nonce-reuse detection operates on; they are intentionally decoupled
// from any particular signature source (a Bitcoin scriptSig, a raw ECDSA
// signature from some other protocol, etc.) so that this tool can be used
// against any dataset of (pubkey, r, s, z) tuples.
type record struct {
	// id is a free-form, human-readable label for the record (e.g. a
	// "<txid>:<input index>" string). It has no bearing on detection and
	// is only used to identify records in the report.
	id string

	// pubKeyHex is the hex-encoded public key, as it appeared in the
	// input, preserved verbatim for reporting purposes.
	pubKeyHex string

	// pubKey is the parsed public key.
	pubKey *btcec.PublicKey

	// r and s are the 32-byte big-endian signature components.
	r, s [32]byte

	// z is the message hash (e.g. sighash) that was signed, exactly as
	// provided in the input.
	z []byte
}

// pubKeyGroupKey returns a string that uniquely identifies the public key
// this record was signed with, suitable for use as a map key when grouping
// records by signer.
func (rec *record) pubKeyGroupKey() string {
	return hex.EncodeToString(rec.pubKey.SerializeCompressed())
}

// rHex returns the hex encoding of the record's R value, suitable for use as
// a map key when grouping records by R value within a signer.
func (rec *record) rHex() string {
	return hex.EncodeToString(rec.r[:])
}

// csvColumns are the recognized header names for the input CSV. Column order
// does not matter, but the header row is required so that columns can be
// identified by name. "sig" is mutually exclusive with the "r"/"s" pair: a
// record must supply exactly one of ("sig") or ("r" and "s").
var csvColumns = struct {
	id, pubKey, sig, r, s, z string
}{
	id:     "id",
	pubKey: "pubkey",
	sig:    "sig",
	r:      "r",
	s:      "s",
	z:      "z",
}

// loadRecords reads and parses the signature records contained in the CSV
// file at path. See the package README for a full description of the
// expected format.
func loadRecords(path string) ([]*record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseRecords(f)
}

// parseRecords parses signature records out of r, which must contain
// well-formed CSV data with a header row.
func parseRecords(r io.Reader) ([]*record, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.Comment = '#'

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	colIdx := make(map[string]int, len(header))
	for i, name := range header {
		colIdx[strings.ToLower(strings.TrimSpace(name))] = i
	}

	requiredCol := func(name string) (int, error) {
		idx, ok := colIdx[name]
		if !ok {
			return 0, fmt.Errorf("missing required column %q", name)
		}
		return idx, nil
	}

	pubKeyIdx, err := requiredCol(csvColumns.pubKey)
	if err != nil {
		return nil, err
	}
	zIdx, err := requiredCol(csvColumns.z)
	if err != nil {
		return nil, err
	}
	idIdx, hasID := colIdx[csvColumns.id]
	sigIdx, hasSig := colIdx[csvColumns.sig]
	rIdx, hasR := colIdx[csvColumns.r]
	sIdx, hasS := colIdx[csvColumns.s]

	if !hasSig && !(hasR && hasS) {
		return nil, fmt.Errorf("input must have either a %q column or "+
			"both %q and %q columns", csvColumns.sig, csvColumns.r,
			csvColumns.s)
	}

	cols := recordColumns{
		idIdx:     idIdx,
		hasID:     hasID,
		pubKeyIdx: pubKeyIdx,
		zIdx:      zIdx,
		sigIdx:    sigIdx,
		hasSig:    hasSig,
		rIdx:      rIdx,
		hasR:      hasR,
		sIdx:      sIdx,
		hasS:      hasS,
	}

	var records []*record
	rowNum := 1 // header was row 1.
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row %d: %w",
				rowNum+1, err)
		}
		rowNum++

		field := func(idx int) string {
			if idx < 0 || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}

		rec, err := parseRecordRow(rowNum, field, cols)
		if err != nil {
			return nil, err
		}

		records = append(records, rec)
	}

	return records, nil
}

// recordColumns holds the resolved column indices for the fields
// parseRecordRow needs to build a record from a single CSV row.
type recordColumns struct {
	idIdx     int
	hasID     bool
	pubKeyIdx int
	zIdx      int
	sigIdx    int
	hasSig    bool
	rIdx      int
	hasR      bool
	sIdx      int
	hasS      bool
}

func parseRecordRow(rowNum int, field func(int) string, cols recordColumns) (*record, error) {
	pubKeyHex := field(cols.pubKeyIdx)
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid pubkey hex: %w", rowNum, err)
	}
	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid pubkey: %w", rowNum, err)
	}

	zHex := field(cols.zIdx)
	z, err := hex.DecodeString(zHex)
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid z (message hash) hex: %w",
			rowNum, err)
	}
	if len(z) == 0 {
		return nil, fmt.Errorf("row %d: z (message hash) is empty", rowNum)
	}

	rec := &record{
		pubKeyHex: pubKeyHex,
		pubKey:    pubKey,
		z:         z,
	}
	if cols.hasID {
		rec.id = field(cols.idIdx)
	} else {
		rec.id = fmt.Sprintf("row-%d", rowNum)
	}

	sigHex := strings.TrimSpace(field(cols.sigIdx))
	if cols.hasSig && sigHex != "" {
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid sig hex: %w", rowNum, err)
		}
		sig, err := ecdsa.ParseDERSignature(sigBytes)
		if err != nil {
			// Fall back to the more permissive BER parser since not
			// every real-world signature is strict DER.
			sig, err = ecdsa.ParseSignature(sigBytes)
			if err != nil {
				return nil, fmt.Errorf("row %d: invalid sig: %w",
					rowNum, err)
			}
		}
		rec.r, rec.s = ecdsa.ComponentsFromSignature(sig)

		return rec, nil
	}

	if !cols.hasR || !cols.hasS {
		return nil, fmt.Errorf("row %d: no sig and missing r/s columns", rowNum)
	}

	rBytes, err := hex.DecodeString(field(cols.rIdx))
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid r hex: %w", rowNum, err)
	}
	sBytes, err := hex.DecodeString(field(cols.sIdx))
	if err != nil {
		return nil, fmt.Errorf("row %d: invalid s hex: %w", rowNum, err)
	}
	if len(rBytes) == 0 || len(sBytes) == 0 {
		return nil, fmt.Errorf("row %d: r and s must both be provided", rowNum)
	}
	if len(rBytes) > 32 || len(sBytes) > 32 {
		return nil, fmt.Errorf("row %d: r and s must each be at most 32 bytes",
			rowNum)
	}
	copy(rec.r[32-len(rBytes):], rBytes)
	copy(rec.s[32-len(sBytes):], sBytes)

	return rec, nil
}

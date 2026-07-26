// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// hardened returns i with the hardened-derivation bit set, per BIP32.
func hardened(i uint32) uint32 {
	return i + hdkeychain.HardenedKeyStart
}

// addressDeriver turns a public key into the address type a given
// derivation scheme expects (P2PKH, P2SH-P2WPKH, P2WPKH, or P2TR).
type addressDeriver func(pubKey *btcec.PublicKey, net *chaincfg.Params) (btcutil.Address, error)

// AddressScheme describes one of the standard BIP44-family derivation
// schemes: a BIP purpose number and the address type it derives, following
// the convention m/purpose'/coin'/account'/chain/index.
type AddressScheme struct {
	// Key is the short identifier used on the command line and in
	// reports, e.g. "84".
	Key string

	// Purpose is the BIP purpose field (the first hardened path
	// component), e.g. 84 for BIP84.
	Purpose uint32

	// Label is a human-readable description of the scheme, shown in
	// reports.
	Label string

	deriveAddress addressDeriver
}

// allSchemes lists every derivation scheme bip39scan knows how to derive
// and check, keyed by the same short identifier used in AddressScheme.Key.
// Iterate schemeOrder (or sortedSchemeKeys) rather than this map directly
// when a stable order matters.
var allSchemes = map[string]AddressScheme{
	"44": {
		Key:           "44",
		Purpose:       44,
		Label:         "BIP44 Legacy (P2PKH)",
		deriveAddress: deriveP2PKH,
	},
	"49": {
		Key:           "49",
		Purpose:       49,
		Label:         "BIP49 Nested SegWit (P2SH-P2WPKH)",
		deriveAddress: deriveP2SHP2WPKH,
	},
	"84": {
		Key:           "84",
		Purpose:       84,
		Label:         "BIP84 Native SegWit (P2WPKH)",
		deriveAddress: deriveP2WPKH,
	},
	"86": {
		Key:           "86",
		Purpose:       86,
		Label:         "BIP86 Taproot (P2TR)",
		deriveAddress: deriveP2TR,
	},
}

// defaultSchemeOrder is the order schemes are scanned in when the user asks
// for "all" of them, matching the order the corresponding BIPs were
// introduced.
var defaultSchemeOrder = []string{"44", "49", "84", "86"}

// ParseSchemes resolves a comma-separated list of scheme keys (e.g.
// "44,84") into their AddressScheme definitions, preserving the canonical
// (defaultSchemeOrder) ordering regardless of the order keys were listed
// in. The special value "all" (the default) expands to every known scheme.
func ParseSchemes(spec string) ([]AddressScheme, error) {
	if spec == "all" || spec == "" {
		out := make([]AddressScheme, 0, len(defaultSchemeOrder))
		for _, key := range defaultSchemeOrder {
			out = append(out, allSchemes[key])
		}
		return out, nil
	}

	requested := make(map[string]bool)
	for _, field := range strings.Split(spec, ",") {
		key := strings.TrimSpace(field)
		if key == "" {
			continue
		}
		if _, ok := allSchemes[key]; !ok {
			return nil, fmt.Errorf("unknown scheme %q; valid schemes are "+
				"44, 49, 84, 86, or all", key)
		}
		requested[key] = true
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("no schemes requested")
	}

	out := make([]AddressScheme, 0, len(requested))
	for _, key := range defaultSchemeOrder {
		if requested[key] {
			out = append(out, allSchemes[key])
		}
	}
	return out, nil
}

// CoinType returns the SLIP-44 coin type used as the second path
// component: 0' for mainnet-family networks, 1' for all testing networks,
// per convention (BIP44 itself only reserves 0 for "Bitcoin" and 1 for "all
// testnets").
func CoinType(net *chaincfg.Params) uint32 {
	if net.Net == chaincfg.MainNetParams.Net {
		return 0
	}
	return 1
}

// DerivePath walks down from master along m/purpose'/coinType'/account'/
// chain/index, where purpose and coinType are hardened per BIP44 and
// account is hardened per the same convention; chain and index are not
// hardened so that watch-only (public-key-only) wallets could derive the
// same addresses.
func DerivePath(
	master *hdkeychain.ExtendedKey,
	purpose, coinType, account, chain, index uint32,
) (*hdkeychain.ExtendedKey, error) {

	path := []uint32{
		hardened(purpose),
		hardened(coinType),
		hardened(account),
		chain,
		index,
	}

	key := master
	for _, step := range path {
		var err error
		key, err = key.Derive(step)
		if err != nil {
			return nil, fmt.Errorf("failed to derive step %d: %w", step,
				err)
		}
	}
	return key, nil
}

// deriveP2PKH derives the legacy pay-to-pubkey-hash address for pubKey, as
// used by BIP44.
func deriveP2PKH(pubKey *btcec.PublicKey, net *chaincfg.Params) (btcutil.Address, error) {
	pkHash := btcutil.Hash160(pubKey.SerializeCompressed())
	return btcutil.NewAddressPubKeyHash(pkHash, net)
}

// deriveP2WPKH derives the native SegWit (bech32) pay-to-witness-pubkey-hash
// address for pubKey, as used by BIP84.
func deriveP2WPKH(pubKey *btcec.PublicKey, net *chaincfg.Params) (btcutil.Address, error) {
	pkHash := btcutil.Hash160(pubKey.SerializeCompressed())
	return btcutil.NewAddressWitnessPubKeyHash(pkHash, net)
}

// deriveP2SHP2WPKH derives the P2SH-nested SegWit address for pubKey, as
// used by BIP49: a P2WPKH witness program wrapped in a P2SH redeem script.
func deriveP2SHP2WPKH(pubKey *btcec.PublicKey, net *chaincfg.Params) (btcutil.Address, error) {
	witnessAddr, err := deriveP2WPKH(pubKey, net)
	if err != nil {
		return nil, err
	}

	redeemScript, err := txscript.PayToAddrScript(witnessAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to build witness redeem script: %w",
			err)
	}

	return btcutil.NewAddressScriptHash(redeemScript, net)
}

// deriveP2TR derives the key-path-only Taproot address for pubKey, as used
// by BIP86. No script-path commitments are included, matching how BIP86
// wallets derive their default receive/change addresses.
func deriveP2TR(pubKey *btcec.PublicKey, net *chaincfg.Params) (btcutil.Address, error) {
	outputKey := txscript.ComputeTaprootKeyNoScript(pubKey)
	witnessProg := schnorr.SerializePubKey(outputKey)
	return btcutil.NewAddressTaproot(witnessProg, net)
}

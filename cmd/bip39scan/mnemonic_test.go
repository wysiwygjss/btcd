// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

// TestSeedFromMnemonicVectors checks SeedFromMnemonic (and, transitively,
// pbkdf2HMACSHA512) against the official BIP39 English test vectors from
// https://github.com/trezor/python-mnemonic/blob/master/vectors.json, all of
// which use the passphrase "TREZOR". It also verifies that feeding the
// derived seed into BIP32's NewMaster reproduces the expected root xprv,
// exercising the full mnemonic -> seed -> master key pipeline end to end.
func TestSeedFromMnemonicVectors(t *testing.T) {
	const passphrase = "TREZOR"

	testCases := []struct {
		name     string
		mnemonic string
		wantSeed string
		wantXprv string
	}{
		{
			name:     "12 words, all abandon",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
			wantSeed: "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e53495531f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04",
			wantXprv: "xprv9s21ZrQH143K3h3fDYiay8mocZ3afhfULfb5GX8kCBdno77K4HiA15Tg23wpbeF1pLfs1c5SPmYHrEpTuuRhxMwvKDwqdKiGJS9XFKzUsAF",
		},
		{
			name:     "12 words, legal winner",
			mnemonic: "legal winner thank year wave sausage worth useful legal winner thank yellow",
			wantSeed: "2e8905819b8723fe2c1d161860e5ee1830318dbf49a83bd451cfb8440c28bd6fa457fe1296106559a3c80937a1c1069be3a3a5bd381ee6260e8d9739fce1f607",
			wantXprv: "xprv9s21ZrQH143K2gA81bYFHqU68xz1cX2APaSq5tt6MFSLeXnCKV1RVUJt9FWNTbrrryem4ZckN8k4Ls1H6nwdvDTvnV7zEXs2HgPezuVccsq",
		},
		{
			name:     "12 words, letter advice",
			mnemonic: "letter advice cage absurd amount doctor acoustic avoid letter advice cage above",
			wantSeed: "d71de856f81a8acc65e6fc851a38d4d7ec216fd0796d0a6827a3ad6ed5511a30fa280f12eb2e47ed2ac03b5c462a0358d18d69fe4f985ec81778c1b370b652a8",
			wantXprv: "xprv9s21ZrQH143K2shfP28KM3nr5Ap1SXjz8gc2rAqqMEynmjt6o1qboCDpxckqXavCwdnYds6yBHZGKHv7ef2eTXy461PXUjBFQg6PrwY4Gzq",
		},
		{
			name:     "12 words, all zoo",
			mnemonic: "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong",
			wantSeed: "ac27495480225222079d7be181583751e86f571027b0497b5b5d11218e0a8a13332572917f0f8e5a589620c6f15b11c61dee327651a14c34e18231052e48c069",
			wantXprv: "xprv9s21ZrQH143K2V4oox4M8Zmhi2Fjx5XK4Lf7GKRvPSgydU3mjZuKGCTg7UPiBUD7ydVPvSLtg9hjp7MQTYsW67rZHAXeccqYqrsx8LcXnyd",
		},
		{
			name:     "18 words, real-word sentence",
			mnemonic: "gravity machine north sort system female filter attitude volume fold club stay feature office ecology stable narrow fog",
			wantSeed: "628c3827a8823298ee685db84f55caa34b5cc195a778e52d45f59bcf75aba68e4d7590e101dc414bc1bbd5737666fbbef35d1f1903953b66624f910feef245ac",
			wantXprv: "xprv9s21ZrQH143K3uT8eQowUjsxrmsA9YUuQQK1RLqFufzybxD6DH6gPY7NjJ5G3EPHjsWDrs9iivSbmvjc9DQJbJGatfa9pv4MZ3wjr8qWPAK",
		},
		{
			name:     "24 words, real-word sentence",
			mnemonic: "hamster diagram private dutch cause delay private meat slide toddler razor book happy fancy gospel tennis maple dilemma loan word shrug inflict delay length",
			wantSeed: "64c87cde7e12ecf6704ab95bb1408bef047c22db4cc7491c4271d170a1b213d20b385bc1588d9c7b38f1b39d415665b8a9030c9ec653d75e65f847d8fc1fc440",
			wantXprv: "xprv9s21ZrQH143K2XTAhys3pMNcGn261Fi5Ta2Pw8PwaVPhg3D8DWkzWQwjTJfskj8ofb81i9NP2cUNKxwjueJHHMQAnxtivTA75uUFqPFeWzk",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateMnemonic(tc.mnemonic); err != nil {
				t.Fatalf("ValidateMnemonic() returned unexpected error: %v",
					err)
			}

			seed, err := SeedFromMnemonic(tc.mnemonic, passphrase)
			if err != nil {
				t.Fatalf("SeedFromMnemonic() returned unexpected error: %v",
					err)
			}

			wantSeed, err := hex.DecodeString(tc.wantSeed)
			if err != nil {
				t.Fatalf("failed to decode expected seed: %v", err)
			}
			if !equalBytes(seed, wantSeed) {
				t.Fatalf("seed mismatch:\ngot:  %x\nwant: %x", seed,
					wantSeed)
			}

			master, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("NewMaster() returned unexpected error: %v", err)
			}
			if got := master.String(); got != tc.wantXprv {
				t.Fatalf("root xprv mismatch:\ngot:  %s\nwant: %s", got,
					tc.wantXprv)
			}
		})
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestValidateMnemonicInvalid checks that mnemonics with a bad word count,
// unknown words, or a corrupted checksum are all rejected.
func TestValidateMnemonicInvalid(t *testing.T) {
	testCases := []struct {
		name     string
		mnemonic string
	}{
		{
			name:     "wrong word count",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon",
		},
		{
			name:     "unknown word",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon notaword",
		},
		{
			name: "bad checksum",
			// Same words as the "all abandon" vector, but the last word
			// (which carries the checksum) is swapped for a different
			// valid wordlist entry, so the checksum no longer matches.
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon zoo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateMnemonic(tc.mnemonic); err == nil {
				t.Fatalf("ValidateMnemonic(%q) unexpectedly succeeded",
					tc.mnemonic)
			}
		})
	}
}

// TestValidateMnemonicCaseInsensitive checks that mnemonic words are matched
// against the wordlist case-insensitively for checksum validation purposes.
//
// NOTE: word matching is case-insensitive, but (matching the reference
// python-mnemonic implementation) the exact bytes of the mnemonic sentence
// as given are still what feed into the PBKDF2 seed derivation, so a
// mixed-case mnemonic is not guaranteed to produce the same seed as its
// all-lowercase form. Always use the exact casing your wallet generated.
func TestValidateMnemonicCaseInsensitive(t *testing.T) {
	mnemonic := "Abandon Abandon Abandon Abandon Abandon Abandon Abandon Abandon Abandon Abandon Abandon About"
	if err := ValidateMnemonic(mnemonic); err != nil {
		t.Fatalf("ValidateMnemonic() returned unexpected error: %v", err)
	}

	if _, err := SeedFromMnemonic(mnemonic, "TREZOR"); err != nil {
		t.Fatalf("SeedFromMnemonic() returned unexpected error: %v", err)
	}
}

// TestEnglishWordlistSize sanity-checks the embedded wordlist.
func TestEnglishWordlistSize(t *testing.T) {
	if len(englishWordlist) != 2048 {
		t.Fatalf("englishWordlist has %d entries, want 2048",
			len(englishWordlist))
	}
	if len(englishWordIndex) != 2048 {
		t.Fatalf("englishWordIndex has %d entries, want 2048",
			len(englishWordIndex))
	}
	for i, word := range englishWordlist {
		idx, ok := englishWordIndex[word]
		if !ok {
			t.Fatalf("word %q missing from englishWordIndex", word)
		}
		if int(idx) != i {
			t.Fatalf("word %q has index %d in map, want %d", word, idx, i)
		}
	}
}

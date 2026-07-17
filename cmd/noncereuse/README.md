# noncereuse

`noncereuse` scans a dataset of ECDSA signatures for the classic **nonce
reuse** (a.k.a. "duplicate R") vulnerability and recovers the private key
behind any pair of signatures it finds.

## Background

An ECDSA signature `(r, s)` over a message with hash `e`, produced with
private key `d` and a secret, per-signature nonce `k`, satisfies:

```
r = (k*G).x mod N
s = k^-1 * (e + d*r) mod N
```

If the *same* nonce `k` is ever reused to sign two *different* messages
with the same key -- which should never happen if the nonce is generated
correctly (e.g. via RFC 6979 or a cryptographically secure random number
generator), but has happened repeatedly in the real world due to broken RNGs
-- the resulting signatures `(r, s1)` and `(r, s2)` share the same `r`. Given
the two signatures and the two message hashes, anyone can then recover the
nonce and the private key with nothing more than a few modular arithmetic
operations:

```
k = (e1 - e2) / (s1 - s2) mod N
d = (s1*k - e1) / r        mod N
```

This has caused real losses of funds in the wild, most notably the 2013
Android Bitcoin wallet bug (a broken `SecureRandom` that occasionally
repeated a previous nonce) and the 2023 "Milk Sad" vulnerability in several
wallet libraries that used a weak/predictable seed for nonce generation. The
same failure mode applies to any ECDSA usage, not just Bitcoin.

The recovery math itself lives in the
[`btcec/ecdsa`](../../btcec/ecdsa/noncereuse.go) package
(`RecoverPrivateKeyFromDuplicateR` and `RecoverPrivateKeyFromSignatures`);
this tool is a small CLI wrapper that applies it across a whole dataset of
signatures at once.

## Usage

```
go run ./cmd/noncereuse -input signatures.csv [-output report.txt] [-testnet]
```

| Flag        | Default  | Description                                                             |
|-------------|----------|--------------------------------------------------------------------------|
| `-input`    | *(none)* | Path to the input CSV file of signature records (required).            |
| `-output`   | stdout   | Path to write the report to.                                           |
| `-testnet`  | `false`  | Render recovered keys/addresses using testnet encoding instead of mainnet. |

Exit status is non-zero if the input could not be read/parsed or an
unexpected internal error occurred; a report of "no nonce reuse detected" is
not treated as an error.

## Input format

The input is a CSV file with a header row. The following columns are
recognized (column order does not matter):

| Column   | Required?          | Description                                                        |
|----------|---------------------|----------------------------------------------------------------------|
| `id`     | no                  | A free-form label for the record (e.g. `<txid>:<input index>`), used only in the report. |
| `pubkey` | yes                 | Hex-encoded public key (compressed or uncompressed) that produced the signature. |
| `sig`    | one of `sig` or (`r` and `s`) | Hex-encoded DER (or BER) signature. |
| `r`      | one of `sig` or (`r` and `s`) | Hex-encoded signature `R` value. |
| `s`      | one of `sig` or (`r` and `s`) | Hex-encoded signature `S` value. |
| `z`      | yes                 | Hex-encoded message hash (e.g. sighash) that was signed.            |

Lines whose first character is `#` are treated as comments and ignored,
which is useful for annotating example/sample files.

Note that this tool intentionally operates on an already-extracted dataset
of `(pubkey, r, s, z)` (or `(pubkey, sig, z)`) tuples rather than on raw
transactions: extracting signatures and computing the correct sighash from a
raw transaction depends on the specific script/segwit version involved,
which is outside the scope of this generic detector. Any process that
produces such a dataset -- for example, iterating over a block's
transactions and pulling the signature(s) and computed sighash out of each
input's scriptSig/witness -- can feed this tool.

See [`testdata/sample.csv`](testdata/sample.csv) for a worked example: it
contains two signatures produced with a deliberately reused nonce (and thus
recoverable), plus one unrelated signature that is not.

## Example

```
$ go run ./cmd/noncereuse -input cmd/noncereuse/testdata/sample.csv
2026/07/17 04:42:15 loaded 3 signature record(s) from cmd/noncereuse/testdata/sample.csv
Recovered 1 private key(s) from reused nonces:

[1] public key : 03ab0b34bcb759f24b2bb8c4ca3365f32b700a7a96a78d40906b4da06a43c56bfd
    address    : 1D1Ems1rdCnxsGXnQfwRqy2m68ksQaSoRb
    shared r   : fa19b5301b0d38ba2b3017643cba28dd8d0e12f49f3db3fd2729ecb183c99971
    from       : payment-to-alice, payment-to-bob
    private key (hex): 3f719c1a8b2d4e6f015a9d3c7e2b8f146327a5d90e4c81fb356a1d8e72c40953
    private key (WIF): KyM37nGsmbDMtgncLLRFzJSMmBHqRijhgo2CsQSYXct9u5gh6YEJ
------------------------------------------------------------
```

## Handling of canonicalized ("low-S") signatures

Both `s` and `N - s` are valid signatures for a given `(message, key, r)`;
Bitcoin (per BIP0062) and most other ECDSA implementations always emit the
"low-S" (`s <= N/2`) form. Because that normalization is applied
independently to each signature, two signatures collected from the wild that
truly share a nonce may end up with `s` values that correspond to the same or
to *opposite* signs of that nonce. `RecoverPrivateKeyFromDuplicateR`
transparently tries both possibilities and self-verifies the result, so
`noncereuse` correctly recovers the key either way.

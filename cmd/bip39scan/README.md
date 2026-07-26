# bip39scan

`bip39scan` takes a [BIP39](https://github.com/bitcoin/bips/blob/master/bip-0039.mediawiki)
mnemonic seed phrase and checks whether it controls any funds, across every
standard derivation path wallets commonly use:

| Scheme | Path template            | Address type                  |
|--------|---------------------------|--------------------------------|
| BIP44  | `m/44'/coin'/acct'/chg/i` | Legacy (`1...`)                |
| BIP49  | `m/49'/coin'/acct'/chg/i` | Nested SegWit (`3...`)         |
| BIP84  | `m/84'/coin'/acct'/chg/i` | Native SegWit (`bc1q...`)      |
| BIP86  | `m/86'/coin'/acct'/chg/i` | Taproot (`bc1p...`)            |

This is useful when recovering an old or unfamiliar wallet backup whose
exact derivation scheme is unknown: rather than guessing, `bip39scan`
derives addresses along all four schemes (and as many accounts/indices as
needed) and checks each one's on-chain balance in one pass.

## What it does

For every requested scheme and account, `bip39scan` walks each of the
`external` (receive) and `internal` (change) branches index by index,
deriving that index's address and looking up its balance. It stops walking
a branch once it sees `-gap-limit` (default 20) consecutive addresses with
no on-chain activity — the same
["address gap limit" convention BIP44 itself defines](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki#address-gap-limit)
and that wallets such as Electrum use for account discovery — so a scan
naturally terminates instead of checking addresses forever. Branches are
scanned concurrently (`-concurrency` balance lookups in flight at once).

At the end, it prints a report: every address with any on-chain activity
(confirmed balance, unconfirmed/mempool balance, and transaction count),
plus totals.

## Privacy

Only the addresses `bip39scan` derives are ever sent over the network (to
look up their balance). The mnemonic, passphrase, seed, and every derived
private/public key always stay local — they are never transmitted.

That said, sending a wallet's own addresses to a third-party block explorer
still leaks information about that wallet to that third party. To avoid
this entirely:

* Pass `-offline` to skip network access altogether; `bip39scan` will just
  derive and list the addresses it would have checked (`-gap-limit` per
  branch), so you can check them elsewhere (e.g. your own full node).
* Point `-rpc-host` at a local [Bitcoin Core](https://bitcoincore.org/)
  (`bitcoind`) node so lookups never leave your machine (see below).
* Or point `-api-base` at a block explorer you run yourself (any
  [Esplora](https://github.com/Blockstream/electrs)-compatible instance,
  such as one backed by `electrs`).

## Bitcoin Core RPC mode

By default, `bip39scan` queries an Esplora-compatible REST API
(`blockstream.info` unless you override `-api-base`). To query your own
synced `bitcoind` instead, pass `-rpc-host`:

```sh
# Mainnet bitcoind on the default RPC port, cookie-file auth:
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt \
  -rpc-host localhost:8332 \
  -rpc-cookie-file ~/.bitcoin/.cookie

# Testnet:
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt -testnet \
  -rpc-host localhost:18332 \
  -rpc-cookie-file ~/.bitcoin/testnet3/.cookie

# Unix socket RPC (common on Linux):
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt \
  -rpc-host unix:///path/to/bitcoin/.cookie \
  -rpc-cookie-file ~/.bitcoin/.cookie
```

Under the hood, RPC mode uses two Bitcoin Core calls per address:

1. `scanblocks` (when `-rpc-scan-blocks` is enabled and the node was
   started with `-blockfilterindex=1`) to detect whether the address ever
   appeared in a confirmed transaction — including addresses that have since
   been fully spent, which the gap-limit scan needs to see.
2. `scantxoutset` to read any current UTXOs and confirmed balance.

RPC mode is **much slower** than an Esplora indexer because each address
may require scanning the node's compact block filters and/or UTXO set.
Mempool (unconfirmed) balances are not reported. For the best of both
worlds (local + fast), run [electrs](https://github.com/Blockstream/electrs)
locally and point `-api-base` at it instead.

`-rpc-host` and `-api-base` are mutually exclusive.

## Usage

```sh
# From the repository root — prefer -mnemonic-file over -mnemonic to avoid
# leaving the seed phrase in your shell history or process list:
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt

# Only check native SegWit (BIP84) and Taproot (BIP86), 2 accounts each:
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt \
  -schemes 84,86 -accounts 2

# Derive addresses only, with no network access at all:
go run ./cmd/bip39scan -mnemonic-file ./my-seed-phrase.txt -offline

# A raw seed (e.g. recovered from an old xprv) instead of a mnemonic:
go run ./cmd/bip39scan -seed-hex 000102030405060708090a0b0c0d0e0f
```

| Flag             | Default                  | Description |
|-------------------|---------------------------|-------------|
| `-mnemonic`       | (none)                    | BIP39 mnemonic seed phrase. |
| `-mnemonic-file`  | (none)                    | Path to a file containing the mnemonic (safer than `-mnemonic`). |
| `-passphrase`     | `""`                      | Optional BIP39 passphrase (the "25th word"). |
| `-seed-hex`       | (none)                    | Use this hex seed directly instead of deriving one from a mnemonic. |
| `-testnet`        | `false`                   | Derive testnet3 addresses instead of mainnet. |
| `-schemes`        | `all`                     | Comma-separated: `44`, `49`, `84`, `86`, or `all`. |
| `-accounts`       | `1`                       | Number of accounts to scan per scheme. |
| `-scan-change`    | `true`                    | Also scan each account's internal/change chain. |
| `-gap-limit`      | `20`                      | Consecutive unused addresses before a branch is considered exhausted. |
| `-max-index`      | `1000`                    | Hard cap on addresses checked per branch, regardless of `-gap-limit`. |
| `-concurrency`    | `4`                       | Max balance lookups in flight at once. |
| `-timeout`        | `15s`                     | Per-request timeout for balance lookups. |
| `-api-base`       | blockstream.info's API    | Base URL of an Esplora-compatible balance API. |
| `-rpc-host`       | (none)                    | Query balances via local `bitcoind` RPC instead of Esplora. |
| `-rpc-user`       | (none)                    | `bitcoind` RPC username (ignored when `-rpc-cookie-file` is set). |
| `-rpc-pass`       | (none)                    | `bitcoind` RPC password (ignored when `-rpc-cookie-file` is set). |
| `-rpc-cookie-file`| (none)                    | Path to `bitcoind`'s `.cookie` file for RPC auth. |
| `-rpc-disable-tls`| `true`                    | Use plain HTTP for `bitcoind` RPC (the default listener). |
| `-rpc-timeout`    | `10m`                     | Per-request timeout for `bitcoind` RPC calls. |
| `-rpc-scan-blocks`| `true`                    | Use `scanblocks` to detect fully-spent addresses (needs `-blockfilterindex=1`). |
| `-max-retries`    | `3`                       | Retries after a transient failure (network error, HTTP 429/5xx). |
| `-offline`        | `false`                   | Skip balance checks; just derive and list addresses. |
| `-quiet`          | `false`                   | Suppress per-address progress logging. |
| `-out`            | (none)                    | Also write the final report to this file. |

## Files

* `mnemonic.go` — BIP39 mnemonic validation (wordlist + checksum) and
  seed derivation (PBKDF2-HMAC-SHA512), verified against the official
  [BIP39 test vectors](https://github.com/trezor/python-mnemonic/blob/master/vectors.json).
* `paths.go` — BIP32 path derivation and per-scheme address encoding,
  verified against the worked examples in the BIP44/49/84/86 specs.
* `scanner.go` — the concurrent, gap-limit-aware multi-branch scan.
* `balance.go` — the Esplora-compatible HTTP balance-lookup client.
* `report.go` — report/address-list formatting.

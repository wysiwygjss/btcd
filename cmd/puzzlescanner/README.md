# puzzlescanner

`puzzlescanner` randomly scans the chunks of a large private-key keyspace —
such as the [Bitcoin Puzzle #71](https://privatekeys.pw/puzzles/bitcoin-puzzle-tx)
challenge, whose 10,000-chunk keyspace ships in
[`testdata/puzzle71_10000.txt`](./testdata/puzzle71_10000.txt) — looking for
the private key behind a target P2PKH address.

Each chunk in the ranges file is a `<start-hex>:<end-hex>` line describing a
sub-range of the full keyspace; its 1-based line number is used as the
"segment name" (e.g. line 25 is segment `25`).

## What it does

On every run, the tool:

1. Loads the full list of chunks (segments) from the ranges file.
2. Compares that list against the `<id>.scannedrandom.txt` result files
   already present in the output directory, so a chunk that has already been
   scanned (by this run or an earlier one) is never scanned again.
3. Picks one of the remaining, not-yet-scanned chunks at random.
4. Randomly samples private keys from within that chunk (using
   `crypto/rand` for unbiased sampling across the full sub-range) for up to
   `-duration` (default `30m`), deriving each candidate's P2PKH address and
   comparing it against the target.
5. Writes the outcome — hex range, date/time the scan started and ended,
   keys tried, and whether a match was found — to `<id>.scannedrandom.txt`
   in the output directory.
6. Randomly jumps to a new, still-unscanned chunk and repeats, until every
   chunk has been scanned, a match is found, or the process is interrupted
   (Ctrl+C).

If a match is ever found, the private key (hex and WIF) is printed and saved
to that chunk's result file, and the tool exits immediately.

Multiple instances of `puzzlescanner` can safely point at the same output
directory (e.g. a shared/networked folder): each instance atomically
reserves a chunk with a `<id>.scannedrandom.txt.inprogress` lock file before
scanning it, so two instances never scan the same chunk concurrently. A lock
left behind by a crashed or killed run is treated as abandoned — and made
available again — once it is older than `duration * -stale-lock-multiplier`
(2x the chunk duration by default).

## Usage

```sh
# From the repository root:
go run ./cmd/puzzlescanner \
  -ranges cmd/puzzlescanner/testdata/puzzle71_10000.txt \
  -address 1PWo3JeB9jrGwfHDNpdGK54CRas7fsVzXU \
  -outdir scannedchunks \
  -duration 30m
```

All flags are optional; the defaults already point at Puzzle #71's 10,000
chunks and address.

| Flag                        | Default                                          | Description |
|-----------------------------|---------------------------------------------------|-------------|
| `-ranges`                   | `cmd/puzzlescanner/testdata/puzzle71_10000.txt`   | Chunk list file (`<start-hex>:<end-hex>` per line). |
| `-address`                  | `1PWo3JeB9jrGwfHDNpdGK54CRas7fsVzXU`               | Target P2PKH address to search for. |
| `-outdir`                   | `scannedchunks`                                    | Directory for `<id>.scannedrandom.txt` result files and lock files. |
| `-duration`                 | `30m`                                              | How long to randomly scan each chunk before jumping to another. |
| `-workers`                  | number of CPUs                                     | Concurrent goroutines sampling keys within a chunk. |
| `-once`                     | `false`                                            | Scan a single random chunk and exit, instead of looping forever. |
| `-stale-lock-multiplier`    | `2.0`                                              | An `.inprogress` lock older than `duration * multiplier` is reclaimed. |

Stop the tool at any time with Ctrl+C; the chunk in progress is abandoned
cleanly (its lock file is removed) so it will be picked up again — by this
run or another — later.

## Result file format

```
Segment (chunk):  25
Range start:      00000000000000000000000000000000000000000000004027525460aa64c2f0
Range end:        00000000000000000000000000000000000000000000004028f5c28f5c28f5b9
Range size:       109951162777601 keys
Target address:   1PWo3JeB9jrGwfHDNpdGK54CRas7fsVzXU
Scan started:     2026-07-14 22:00:00 UTC
Scan ended:       2026-07-14 22:30:00 UTC
Duration:         30m0s
Keys tried:       284739201
Keys/sec:         158188

RESULT:           not found in this segment
```

If a match is found, the file additionally contains the private key (hex and
WIF) and the matched address.

## Notes on realism

Puzzle #71's full keyspace is ~2^70 keys; each of the 10,000 chunks shipped
in `testdata/puzzle71_10000.txt` still contains roughly 2^57 keys. Random
sampling — in pure Go, at perhaps 10^5-10^6 keys/sec on a single machine — has
an astronomically small chance of ever landing on the correct key. This tool
is provided as a correctly-behaving scanning/bookkeeping harness (random
chunk selection, no double-scanning, timestamped per-segment reports,
crash-safe locking); it is not a realistic way to actually win the puzzle
bounty without vastly more compute (GPU/FPGA-accelerated secp256k1 kernels
running for a very long time across many machines).

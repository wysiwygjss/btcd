// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strings"
)

// keyRange represents one of the chunks that the full puzzle keyspace has
// been split into. ID is the 1-based line number of the chunk inside the
// ranges file, which is also used as the "segment name" for the
// <ID>.scannedrandom.txt result files.
type keyRange struct {
	ID    int
	Start *big.Int
	End   *big.Int
}

// Size returns the number of keys contained in the range, inclusive of both
// endpoints.
func (r *keyRange) Size() *big.Int {
	size := new(big.Int).Sub(r.End, r.Start)
	return size.Add(size, big.NewInt(1))
}

// loadRanges reads a chunk file where every line has the form
// "<start-hex>:<end-hex>" and returns one keyRange per line, numbered
// starting at 1 in file order.
func loadRanges(path string) ([]*keyRange, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open ranges file: %w", err)
	}
	defer f.Close()

	var ranges []*keyRange
	scanner := bufio.NewScanner(f)
	// Chunk lines are short, but be generous with the buffer just in
	// case the ranges file uses wider hex encodings than expected.
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("ranges file %s: line %d: expected "+
				"\"<start-hex>:<end-hex>\", got %q", path, lineNum, line)
		}

		start, ok := new(big.Int).SetString(strings.TrimSpace(parts[0]), 16)
		if !ok {
			return nil, fmt.Errorf("ranges file %s: line %d: invalid start "+
				"hex value %q", path, lineNum, parts[0])
		}
		end, ok := new(big.Int).SetString(strings.TrimSpace(parts[1]), 16)
		if !ok {
			return nil, fmt.Errorf("ranges file %s: line %d: invalid end "+
				"hex value %q", path, lineNum, parts[1])
		}
		if start.Cmp(end) > 0 {
			return nil, fmt.Errorf("ranges file %s: line %d: start > end",
				path, lineNum)
		}

		ranges = append(ranges, &keyRange{
			ID:    lineNum,
			Start: start,
			End:   end,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("unable to read ranges file: %w", err)
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("ranges file %s contains no chunks", path)
	}

	return ranges, nil
}

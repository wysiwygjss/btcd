// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const timeLayout = "2006-01-02 15:04:05 MST"

// writeReport writes the "<id>.scannedrandom.txt" result file for a
// completed chunk scan, recording the hex range, the date/time the scan
// started and ended, how many keys were tried, and whether a match for the
// target address was found.
func writeReport(outDir string, targetAddr string, res *scanResult) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("unable to create output directory: %w", err)
	}

	path := filepath.Join(outDir, resultFileName(res.Chunk.ID))
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("unable to create report file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "Segment (chunk):  %d\n", res.Chunk.ID)
	fmt.Fprintf(f, "Range start:      %064x\n", res.Chunk.Start)
	fmt.Fprintf(f, "Range end:        %064x\n", res.Chunk.End)
	fmt.Fprintf(f, "Range size:       %s keys\n", res.Chunk.Size().String())
	fmt.Fprintf(f, "Target address:   %s\n", targetAddr)
	fmt.Fprintf(f, "Scan started:     %s\n", res.StartTime.Format(timeLayout))
	fmt.Fprintf(f, "Scan ended:       %s\n", res.EndTime.Format(timeLayout))
	fmt.Fprintf(f, "Duration:         %s\n", res.Duration().Round(time.Second))
	fmt.Fprintf(f, "Keys tried:       %d\n", res.KeysTried)
	fmt.Fprintf(f, "Keys/sec:         %.0f\n", res.KeysPerSecond())

	if res.Found {
		fmt.Fprintf(f, "\nRESULT:           *** MATCH FOUND ***\n")
		fmt.Fprintf(f, "Private key hex:  %064x\n", res.FoundKey)
		fmt.Fprintf(f, "Private key WIF:  %s\n", res.FoundWIF)
		fmt.Fprintf(f, "Matched address:  %s\n", res.FoundAddr)
	} else {
		fmt.Fprintf(f, "\nRESULT:           not found in this segment\n")
	}

	return path, nil
}

// Copyright (c) 2026 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// writeReport writes a human-readable summary of findings to w. net selects
// which network's WIF/address encoding is used when rendering the recovered
// private keys.
func writeReport(w io.Writer, findings []*finding, net *chaincfg.Params) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "No nonce reuse detected: every "+
			"signature observed for a given public key used a "+
			"distinct R value.")
		return err
	}

	fmt.Fprintf(w, "Recovered %d private key(s) from reused nonces:\n\n",
		len(findings))

	for i, f := range findings {
		wif, err := btcutil.NewWIF(f.privKey, net, true)
		if err != nil {
			return fmt.Errorf("failed to encode WIF for finding %d: %w",
				i, err)
		}

		addr, err := btcutil.NewAddressPubKeyHash(
			btcutil.Hash160(f.pubKey.SerializeCompressed()), net,
		)
		if err != nil {
			return fmt.Errorf("failed to derive address for finding "+
				"%d: %w", i, err)
		}

		fmt.Fprintf(w, "[%d] public key : %s\n", i+1, f.pubKeyHex)
		fmt.Fprintf(w, "    address    : %s\n", addr.EncodeAddress())
		fmt.Fprintf(w, "    shared r   : %s\n", f.rHex)
		fmt.Fprintf(w, "    from       : %s, %s\n", f.rec1.id, f.rec2.id)
		fmt.Fprintf(w, "    private key (hex): %x\n", f.privKey.Serialize())
		fmt.Fprintf(w, "    private key (WIF): %s\n", wif.String())
		fmt.Fprintln(w, strings.Repeat("-", 60))
	}

	return nil
}

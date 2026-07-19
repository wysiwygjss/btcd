#!/usr/bin/env python3
import csv
import os
import sys
import time

import requests
from bitcoin.core import CTransaction, b2x, x
from bitcoin.core.script import (
    CScript, SignatureHash, SIGVERSION_BASE, SIGVERSION_WITNESS_V0,
    OP_DUP, OP_HASH160, OP_EQUALVERIFY, OP_CHECKSIG,
)

RPC_HOST = os.environ.get("BITCOIN_RPC_HOST", "127.0.0.1")
RPC_PORT = int(os.environ.get("BITCOIN_RPC_PORT", "8332"))
RPC_USER = os.environ.get("BITCOIN_RPC_USER")
RPC_PASSWORD = os.environ.get("BITCOIN_RPC_PASSWORD")
RPC_COOKIE_FILE = os.environ.get(
    "BITCOIN_RPC_COOKIE", os.path.expanduser("~/.bitcoin/.cookie")
)

OUTPUT_CSV = os.environ.get("DUMP_OUTPUT", "ecdsa_dump.csv")
CHECKPOINT_FILE = os.environ.get("DUMP_CHECKPOINT", "ecdsa_dump.checkpoint")
PROGRESS_EVERY = 500

session = requests.Session()
_id_counter = 0


def get_auth():
    if RPC_USER and RPC_PASSWORD:
        return (RPC_USER, RPC_PASSWORD)
    if os.path.exists(RPC_COOKIE_FILE):
        with open(RPC_COOKIE_FILE) as f:
            user, password = f.read().strip().split(":", 1)
        return (user, password)
    sys.exit("No RPC credentials found.")


AUTH = get_auth()
RPC_URL = f"http://{RPC_HOST}:{RPC_PORT}/"


def rpc_call(method, params=None):
    global _id_counter
    _id_counter += 1
    payload = {"jsonrpc": "1.0", "id": _id_counter, "method": method, "params": params or []}
    resp = session.post(RPC_URL, json=payload, auth=AUTH, timeout=180)
    resp.raise_for_status()
    result = resp.json()
    if result.get("error"):
        raise RuntimeError(f"RPC error on {method}{params}: {result['error']}")
    return result["result"]


def iter_script_pushes(script_bytes):
    i, n = 0, len(script_bytes)
    while i < n:
        opcode = script_bytes[i]
        i += 1
        if 1 <= opcode <= 75:
            yield script_bytes[i:i + opcode]
            i += opcode
        elif opcode == 0x4c:
            length = script_bytes[i]; i += 1
            yield script_bytes[i:i + length]; i += length
        elif opcode == 0x4d:
            length = int.from_bytes(script_bytes[i:i + 2], "little"); i += 2
            yield script_bytes[i:i + length]; i += length
        elif opcode == 0x4e:
            length = int.from_bytes(script_bytes[i:i + 4], "little"); i += 4
            yield script_bytes[i:i + length]; i += length


def looks_like_der_sig(data):
    return len(data) >= 9 and len(data) <= 73 and data[0] == 0x30


def looks_like_pubkey(data):
    return (len(data) == 33 and data[0] in (0x02, 0x03)) or (len(data) == 65 and data[0] == 0x04)


def der_to_rs(der_sig_with_hashtype):
    der = der_sig_with_hashtype[:-1]
    try:
        assert der[0] == 0x30
        idx = 2
        assert der[idx] == 0x02
        idx += 1
        rlen = der[idx]; idx += 1
        r = int.from_bytes(der[idx:idx + rlen], "big"); idx += rlen
        assert der[idx] == 0x02
        idx += 1
        slen = der[idx]; idx += 1
        s = int.from_bytes(der[idx:idx + slen], "big")
        return r, s
    except (AssertionError, IndexError):
        return None


def p2pkh_scriptcode(h160):
    return CScript([OP_DUP, OP_HASH160, h160, OP_EQUALVERIFY, OP_CHECKSIG])


def extract(vin_json, prevout_script_hex):
    witness = vin_json.get("txinwitness")
    scriptsig = bytes.fromhex(vin_json["scriptSig"]["hex"])

    if witness and len(witness) == 2:
        sig = bytes.fromhex(witness[0])
        pubkey = bytes.fromhex(witness[1])
        if not (looks_like_der_sig(sig) and looks_like_pubkey(pubkey)):
            return None
        if len(scriptsig) == 0:
            prevout_script = bytes.fromhex(prevout_script_hex)
            if len(prevout_script) != 22 or prevout_script[0:2] != b"\x00\x14":
                return None
            h160 = prevout_script[2:]
        else:
            pushes = list(iter_script_pushes(scriptsig))
            if len(pushes) != 1 or len(pushes[0]) != 22 or pushes[0][0:2] != b"\x00\x14":
                return None
            h160 = pushes[0][2:]
        return sig, pubkey, p2pkh_scriptcode(h160), SIGVERSION_WITNESS_V0

    if witness:
        return None

    pushes = list(iter_script_pushes(scriptsig))
    if len(pushes) == 2 and looks_like_der_sig(pushes[0]) and looks_like_pubkey(pushes[1]):
        return pushes[0], pushes[1], CScript(x(prevout_script_hex)), SIGVERSION_BASE
    if len(pushes) == 1 and looks_like_der_sig(pushes[0]):
        pk_pushes = list(iter_script_pushes(bytes.fromhex(prevout_script_hex)))
        if len(pk_pushes) == 1 and looks_like_pubkey(pk_pushes[0]):
            return pushes[0], pk_pushes[0], CScript(x(prevout_script_hex)), SIGVERSION_BASE
    return None


def process_block(block_hash, writer):
    block = rpc_call("getblock", [block_hash, 3])
    block_time = block["time"]
    count = 0
    for tx_json in block["tx"]:
        if "hex" not in tx_json:
            continue
        tx = None
        for i, vin in enumerate(tx_json["vin"]):
            if "coinbase" in vin:
                continue
            prevout = vin.get("prevout")
            if not prevout:
                continue
            extracted = extract(vin, prevout["scriptPubKey"]["hex"])
            if not extracted:
                continue
            sig, pubkey, sighash_script, sigversion = extracted
            rs = der_to_rs(sig)
            if not rs:
                continue
            r, s = rs
            hashtype = sig[-1]
            amount_sat = int(round(prevout["value"] * 1e8))
            if tx is None:
                tx = CTransaction.deserialize(x(tx_json["hex"]))
            try:
                msg_hash = SignatureHash(
                    sighash_script, tx, i, hashtype,
                    amount=amount_sat, sigversion=sigversion,
                )
            except Exception:
                continue
            writer.writerow([hex(r), hex(s), pubkey.hex(), tx_json["txid"], b2x(msg_hash), block_time])
            count += 1
    return count


def open_output(path, mode):
    if path.endswith(".gz"):
        import gzip
        return gzip.open(path, mode + "t")
    return open(path, mode, newline="")


def load_checkpoint():
    if os.path.exists(CHECKPOINT_FILE):
        with open(CHECKPOINT_FILE) as f:
            return int(f.read().strip())
    return -1


def save_checkpoint(height):
    with open(CHECKPOINT_FILE, "w") as f:
        f.write(str(height))


def main():
    tip = rpc_call("getblockcount")
    start = load_checkpoint() + 1
    print(f"Chain tip: {tip}. Resuming from height {start}.", file=sys.stderr)

    mode = "a" if start > 0 else "w"
    with open_output(OUTPUT_CSV, mode) as f:
        writer = csv.writer(f, delimiter=";")
        t0 = time.time()
        total_sigs = 0
        for height in range(start, tip + 1):
            block_hash = rpc_call("getblockhash", [height])
            total_sigs += process_block(block_hash, writer)
            if height % PROGRESS_EVERY == 0 or height == tip:
                f.flush()
                save_checkpoint(height)
                elapsed = time.time() - t0
                done = height - start + 1
                rate = done / elapsed if elapsed > 0 else 0
                remaining = (tip - height) / rate if rate > 0 else float("inf")
                print(
                    f"height={height}/{tip} sigs={total_sigs} "
                    f"rate={rate:.1f} blocks/s eta={remaining/60:.1f} min",
                    file=sys.stderr,
                )
    print(f"Done. {total_sigs} signatures written to {OUTPUT_CSV}", file=sys.stderr)


if __name__ == "__main__":
    main()

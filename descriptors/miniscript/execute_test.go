package miniscript

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestExecuteKeyOnlyCorpus takes every key-only expression from the
// differential corpus (no time locks or hash fragments, so it is satisfiable
// with signatures alone), asks the satisfier for a non-malleable witness with
// all keys available, and — whenever a witness is produced — executes it
// through the script engine. This proves the satisfier never emits a witness
// that fails to spend the generated script, exercising the full
// parse -> script -> satisfy -> execute pipeline across thousands of
// expressions.
func TestExecuteKeyOnlyCorpus(t *testing.T) {
	t.Parallel()

	// Keys A..Z from the same secret keys the differential harness uses.
	privKeys := map[string]*btcec.PrivateKey{}
	pubKeys := map[string][]byte{}
	for i := 1; i <= 26; i++ {
		var b [32]byte
		b[31] = byte(i)
		priv, pub := btcec.PrivKeyFromBytes(b[:])
		name := string(rune('A' + i - 1))
		privKeys[name] = priv
		pubKeys[name] = pub.SerializeCompressed()
	}
	lookupVar := func(id string) ([]byte, error) {
		if pk, ok := pubKeys[id]; ok {
			return pk, nil
		}
		return nil, nil
	}

	f, err := os.Open("testdata/props_from_rust.tsv")
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	require.True(t, scanner.Scan()) // header

	// Fragments that need secrets/context beyond signatures.
	needsContext := []string{
		"after(", "older(", "sha256(", "hash256(", "ripemd160(",
		"hash160(",
	}
	usesContext := func(expr string) bool {
		for _, frag := range needsContext {
			if strings.Contains(expr, frag) {
				return true
			}
		}
		return false
	}

	tested, satisfied := 0, 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		expr := strings.SplitN(line, "\t", 2)[0]
		if usesContext(expr) {
			continue
		}
		tested++

		node, err := Parse(expr, P2WSH)
		require.NoErrorf(t, err, "parse %s", expr)

		// Some expressions reuse a key across positions, which the
		// duplicate-key check rejects; skip those.
		if err := node.ApplyVars(lookupVar); err != nil {
			continue
		}

		witnessScript, err := node.Script()
		require.NoErrorf(t, err, "script %s", expr)

		// Precompute the sighash so the sign callback can use it.
		var sighash []byte
		sign := func(pubKey []byte) ([]byte, bool) {
			for name, pub := range pubKeys {
				if !bytes.Equal(pubKey, pub) {
					continue
				}
				sig := ecdsa.Sign(privKeys[name], sighash)
				return append(
					sig.Serialize(),
					byte(txscript.SigHashAll),
				), true
			}
			return nil, false
		}

		// Build the transaction and sighash for this specific script.
		addr, err := address.NewAddressWitnessScriptHash(
			chainhash.HashB(witnessScript),
			&chaincfg.TestNet3Params,
		)
		require.NoError(t, err)
		utxoAmount := int64(999799)
		utxoPkScript, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)
		burnPkScript, err := txscript.NullDataScript(nil)
		require.NoError(t, err)
		tx := wire.MsgTx{
			Version: 2,
			TxIn:    []*wire.TxIn{
				wire.NewTxIn(&wire.OutPoint{}, nil, nil),
			},
			TxOut: []*wire.TxOut{{
				Value:    utxoAmount - 200,
				PkScript: burnPkScript,
			}},
		}
		prevOuts := txscript.NewCannedPrevOutputFetcher(
			utxoPkScript, utxoAmount,
		)
		sigHashes := txscript.NewTxSigHashes(&tx, prevOuts)
		sighash, err = txscript.CalcWitnessSigHash(
			witnessScript, sigHashes, txscript.SigHashAll, &tx, 0,
			utxoAmount,
		)
		require.NoError(t, err)

		witness, err := node.Satisfy(&Satisfier{
			CheckOlder: func(uint32) (bool, error) {
				return false, nil
			},
			CheckAfter: func(uint32) (bool, error) {
				return false, nil
			},
			Sign: sign,
			Preimage: func(string, []byte) ([]byte, bool) {
				return nil, false
			},
		})
		if err != nil {
			// No non-malleable satisfaction available; nothing to
			// execute.
			continue
		}
		satisfied++

		// The produced witness must never exceed the statically
		// computed maximum stack size (validates the stacksize bound
		// end to end).
		require.LessOrEqualf(t, len(witness), node.maxWitnessSize(),
			"witness for %s exceeds computed max stack size", expr)

		tx.TxIn[0].Witness = append(witness, witnessScript)
		engine, err := txscript.NewEngine(
			utxoPkScript, &tx, 0, txscript.StandardVerifyFlags, nil,
			sigHashes, utxoAmount, prevOuts,
		)
		require.NoErrorf(t, err, "engine for %s", expr)
		require.NoErrorf(t, engine.Execute(),
			"witness produced for %s failed to execute", expr)
	}
	require.NoError(t, scanner.Err())

	t.Logf("executed key-only corpus: %d expressions, %d satisfied+"+
		"executed", tested, satisfied)
	require.Positive(t, satisfied)
}

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

		node, err := ParseInsane(expr, P2WSH)
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
			TxIn: []*wire.TxIn{
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

		// Step through the script rather than executing it in one go,
		// to compare the true peak number of stack elements against
		// what the computed execution stack size accounts for: the
		// witness elements plus the elements the script pushes on top
		// of them. This is the value the 1000-element consensus limit
		// is checked against.
		//
		// The peak is allowed to be two elements regardless of the
		// computed bound, because the engine also runs the witness
		// program itself (OP_0 <32-byte script hash>) before it hands
		// the stack to the witness script.
		peak, bound := 0, len(witness)+node.maxExecStackSize()
		for {
			done, err := engine.Step()
			require.NoErrorf(t, err, "witness produced for %s "+
				"failed to execute", expr)

			depth := len(engine.GetStack()) +
				len(engine.GetAltStack())
			peak = max(peak, depth)

			if done {
				break
			}
		}
		require.LessOrEqualf(t, peak, max(2, bound), "execution of "+
			"%s peaked at %d stack elements, more than the %d "+
			"accounted for", expr, peak, bound)
	}
	require.NoError(t, scanner.Err())

	t.Logf("executed key-only corpus: %d expressions, %d satisfied+"+
		"executed", tested, satisfied)
	require.Positive(t, satisfied)
}

// TestSatisfactionSizeCorpus checks the satisfaction size pass against the
// satisfier over the whole corpus: the witness the satisfier produces for an
// expression must never be larger than the maximum the pass computed for it,
// which is the number a fee estimate is built on. Signatures and preimages are
// handed out in exactly the sizes the pass assumes, so the two are directly
// comparable.
func TestSatisfactionSizeCorpus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		file string
		ctx  Context
	}{{
		file: "testdata/props_from_rust.tsv",
		ctx:  P2WSH,
	}, {
		file: "testdata/props_from_rust_tap.tsv",
		ctx:  P2TR,
	}} {

		t.Run(tc.ctx.String(), func(t *testing.T) {
			t.Parallel()

			checkSatisfactionSizes(t, tc.file, tc.ctx)
		})
	}
}

// checkSatisfactionSizes satisfies every expression of the given corpus file
// that can be satisfied and compares the produced witness against the computed
// maximum satisfaction size and element count.
func checkSatisfactionSizes(t *testing.T, file string, ctx Context) {
	// The sizes the satisfaction size pass assumes for the context: a
	// 73-byte ECDSA or 66-byte Schnorr signature and a 33-byte preimage,
	// each including its length prefix.
	sigLen, keyLen := 72, compressedPubKeyLen
	if ctx == P2TR {
		sigLen, keyLen = 65, xOnlyPubKeyLen
	}

	// Every key identifier gets a distinct value, as ApplyVars rejects an
	// expression that resolves two of its keys to the same bytes.
	lookup := func(id string) ([]byte, error) {
		hash := chainhash.HashB([]byte(id))
		key := make([]byte, keyLen)
		copy(key, hash)
		if keyLen == compressedPubKeyLen {
			key[0] = 2
		}

		return key, nil
	}

	satisfier := &Satisfier{
		Sign: func([]byte) ([]byte, bool) {
			return make([]byte, sigLen), true
		},
		Preimage: func(string, []byte) ([]byte, bool) {
			return make([]byte, 32), true
		},
		CheckOlder: func(uint32) (bool, error) {
			return true, nil
		},
		CheckAfter: func(uint32) (bool, error) {
			return true, nil
		},
	}

	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	require.True(t, scanner.Scan()) // header

	checked, exact := 0, 0
	for scanner.Scan() {
		expr := strings.Split(scanner.Text(), "\t")[0]

		node, err := ParseInsane(expr, ctx)
		if err != nil {
			continue
		}
		stated, err := node.MaxSatisfactionSize()
		if err != nil {
			continue
		}
		statedElements, err := node.MaxSatisfactionWitnessElements()
		require.NoError(t, err)
		sane := node.IsSane() == nil

		if err := node.ApplyVars(lookup); err != nil {
			continue
		}
		witness, err := node.Satisfy(satisfier)
		if err != nil {
			continue
		}

		checked++
		produced := witnessSize(witness)
		require.LessOrEqualf(t, produced, stated, "satisfaction of "+
			"%s is %d bytes, more than the computed maximum of %d",
			expr, produced, stated)
		if produced == stated {
			exact++
		}

		// The element count holds for every sane expression. It does
		// not for an insane one, whose satisfaction may take a
		// malleable path: the count of a dissatisfaction is modelled
		// after the canonical (non-malleable) one only, which is all a
		// sane expression can produce.
		if sane {
			require.LessOrEqualf(
				t, len(witness)+1, statedElements,
				"satisfaction of %s has %d elements, more "+
					"than the computed maximum of %d",
				expr, len(witness)+1, statedElements,
			)
		}
	}
	require.NoError(t, scanner.Err())

	t.Logf("satisfied %d %v expressions, %d of them at exactly the "+
		"computed maximum size", checked, ctx, exact)
	require.Positive(t, checked)
}

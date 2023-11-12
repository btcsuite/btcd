package miniscript

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestExecStack checks the execution stack size computation against hand
// computed values. This is the property that feeds the P2TR total stack size
// limit, and is not compared against rust-miniscript (whose value is a looser,
// order-dependent estimate).
func TestExecStack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		miniscript string
		ctx        Context
		exec       int
	}{{
		// c:pk_k pushes the public key.
		miniscript: "pk(A)",
		ctx:        P2WSH,
		exec:       1,
	}, {
		// OP_CHECKMULTISIG has all n keys on the stack.
		miniscript: "multi(2,A,B,C)",
		ctx:        P2WSH,
		exec:       3,
	}, {
		// The left result stays on the stack while the right runs, so
		// max(1, 1+1) = 2.
		miniscript: "and_b(pk(A),s:pk(B))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// Only one branch executes.
		miniscript: "or_i(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       1,
	}, {
		// Two pk checks (1 each) plus the running total: peak 2.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// The two numbers before OP_NUMEQUAL.
		miniscript: "multi_a(2,A,B,C)",
		ctx:        P2TR,
		exec:       2,
	}, {
		// v:pk leaves nothing, then pk runs: max(1, 1) = 1.
		miniscript: "and_v(v:pk(A),pk(B))",
		ctx:        P2TR,
		exec:       1,
	}, {
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2TR,
		exec:       2,
	}}

	for _, tc := range cases {
		node, err := Parse(tc.miniscript, tc.ctx)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)
		require.Equalf(t, tc.exec, node.maxExecStackSize(),
			"exec stack size for %s", tc.miniscript)
	}
}

// tapRedeem builds a taproot output whose only script-path leaf is the given
// P2TR miniscript, then spends it via the script path using a satisfaction
// generated from the miniscript, and runs the script engine. It returns any
// error from sanity checking, satisfaction or execution.
func tapRedeem(t *testing.T, miniscript string,
	lookupVar func(string) ([]byte, error), sequence uint32,
	signable map[string]*btcec.PrivateKey, preimage PreimageFunc) error {

	node, err := Parse(miniscript, P2TR)
	if err != nil {
		return err
	}
	if err := node.IsSane(); err != nil {
		return err
	}
	if err := node.ApplyVars(lookupVar); err != nil {
		return err
	}

	witnessScript, err := node.Script()
	if err != nil {
		return err
	}

	// Use a fixed internal key that is not one of the signer keys.
	var internalBytes [32]byte
	internalBytes[31] = 200
	_, internalKey := btcec.PrivKeyFromBytes(internalBytes[:])

	// Build the taproot output committing to the single tapscript leaf.
	tapLeaf := txscript.NewBaseTapLeaf(witnessScript)
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()
	outputKey := txscript.ComputeTaprootOutputKey(internalKey, rootHash[:])
	utxoPkScript, err := txscript.PayToTaprootScript(outputKey)
	if err != nil {
		return err
	}

	utxoAmount := int64(999799)
	burnPkScript, err := txscript.NullDataScript(nil)
	require.NoError(t, err)

	txInput := wire.NewTxIn(&wire.OutPoint{}, nil, nil)
	txInput.Sequence = sequence
	transaction := wire.MsgTx{
		Version: 2,
		TxIn:    []*wire.TxIn{txInput},
		TxOut: []*wire.TxOut{{
			Value:    utxoAmount - 200,
			PkScript: burnPkScript,
		}},
		LockTime: 0,
	}
	prevOuts := txscript.NewCannedPrevOutputFetcher(
		utxoPkScript, utxoAmount,
	)
	sigHashes := txscript.NewTxSigHashes(&transaction, prevOuts)

	witness, err := node.Satisfy(&Satisfier{
		CheckOlder: func(lockTime uint32) (bool, error) {
			return CheckOlder(
				lockTime, uint32(transaction.Version),
				transaction.TxIn[0].Sequence,
			), nil
		},
		CheckAfter: func(lockTime uint32) (bool, error) {
			return CheckAfter(
				lockTime, transaction.LockTime,
				transaction.TxIn[0].Sequence,
			), nil
		},
		Sign: func(pubKey []byte) ([]byte, bool) {
			priv, ok := signable[hex.EncodeToString(pubKey)]
			if !ok {
				return nil, false
			}

			// A BIP340 Schnorr signature over the tapscript leaf.
			// SigHashDefault yields a bare 64-byte signature.
			sig, err := txscript.RawTxInTapscriptSignature(
				&transaction, sigHashes, 0, utxoAmount,
				utxoPkScript, tapLeaf, txscript.SigHashDefault,
				priv,
			)
			require.NoError(t, err)
			return sig, true
		},
		Preimage: preimage,
	})
	if err != nil {
		return err
	}

	// The tapscript witness is the satisfaction, followed by the leaf
	// script and the control block.
	ctrlBlock := tree.LeafMerkleProofs[0].ToControlBlock(internalKey)
	ctrlBytes, err := ctrlBlock.ToBytes()
	require.NoError(t, err)
	transaction.TxIn[0].Witness = append(witness, witnessScript, ctrlBytes)

	engine, err := txscript.NewEngine(
		utxoPkScript, &transaction, 0, txscript.StandardVerifyFlags,
		nil, sigHashes, utxoAmount, prevOuts,
	)
	if err != nil {
		return err
	}
	return engine.Execute()
}

// TestTapRedeem exercises the full P2TR pipeline (parse -> script -> satisfy ->
// execute) end to end: for a set of Tapscript miniscripts it builds a taproot
// output, spends it via the script path with Schnorr signatures, and runs the
// script engine, asserting the spend succeeds exactly when it should.
func TestTapRedeem(t *testing.T) {
	t.Parallel()

	// Generate signer keys A..E, keyed by their x-only serialization.
	names := []string{"A", "B", "C", "D", "E"}
	privKeys := make(map[string]*btcec.PrivateKey)
	xOnly := make(map[string][]byte)
	for _, name := range names {
		priv, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		privKeys[name] = priv
		xOnly[name] = schnorr.SerializePubKey(priv.PubKey())
	}
	lookupVar := func(id string) ([]byte, error) {
		if k, ok := xOnly[id]; ok {
			return k, nil
		}
		return nil, nil
	}

	// A known preimage and its hashes, for the hash-lock cases.
	preimageBytes := bytes.Repeat([]byte{0x42}, 32)
	sha := chainhash.HashB(preimageBytes)
	preimage := func(hasPreimage bool) PreimageFunc {
		return func(hashFunc string, hash []byte) ([]byte, bool) {
			if !hasPreimage || hashFunc != "sha256" {
				return nil, false
			}
			return preimageBytes, bytes.Equal(hash, sha)
		}
	}
	noPreimage := func(string, []byte) ([]byte, bool) { return nil, false }

	// signableSet builds the x-only-key -> priv map for the named signers.
	signableSet := func(want ...string) map[string]*btcec.PrivateKey {
		m := make(map[string]*btcec.PrivateKey)
		for _, name := range want {
			m[hex.EncodeToString(xOnly[name])] = privKeys[name]
		}
		return m
	}

	shaHex := hex.EncodeToString(sha)

	testCases := []struct {
		name       string
		miniscript string
		sequence   uint32
		signers    []string
		preimage   PreimageFunc
		valid      bool
	}{{
		name:       "single key",
		miniscript: "pk(A)",
		signers:    []string{"A"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "single key wrong signer",
		miniscript: "pk(A)",
		signers:    []string{"B"},
		preimage:   noPreimage,
		valid:      false,
	}, {
		name:       "pkh",
		miniscript: "pkh(A)",
		signers:    []string{"A"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "2-of-3 multi_a",
		miniscript: "multi_a(2,A,B,C)",
		signers:    []string{"A", "C"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "2-of-3 multi_a insufficient",
		miniscript: "multi_a(2,A,B,C)",
		signers:    []string{"B"},
		preimage:   noPreimage,
		valid:      false,
	}, {
		name:       "2-of-3 multi_a extra signer",
		miniscript: "multi_a(2,A,B,C)",
		signers:    []string{"A", "B", "C"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "or_d first branch",
		miniscript: "or_d(pk(A),pk(B))",
		signers:    []string{"A"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "or_d second branch",
		miniscript: "or_d(pk(A),pk(B))",
		signers:    []string{"B"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "and_v two keys",
		miniscript: "and_v(v:pk(A),pk(B))",
		signers:    []string{"A", "B"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "thresh of key checks",
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		signers:    []string{"A", "B"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "and_v with sha256 preimage",
		miniscript: "and_v(v:pk(A),sha256(" + shaHex + "))",
		signers:    []string{"A"},
		preimage:   preimage(true),
		valid:      true,
	}, {
		name:       "and_v with sha256 missing preimage",
		miniscript: "and_v(v:pk(A),sha256(" + shaHex + "))",
		signers:    []string{"A"},
		preimage:   preimage(false),
		valid:      false,
	}, {
		name: "andor with timelock else branch",
		miniscript: "andor(multi_a(1,A),older(2)," +
			"multi_a(1,B))",
		sequence: 10,
		signers:  []string{"B"},
		preimage: noPreimage,
		valid:    true,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tapRedeem(
				t, tc.miniscript, lookupVar, tc.sequence,
				signableSet(tc.signers...), tc.preimage,
			)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

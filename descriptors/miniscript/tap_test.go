package miniscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestExecStack checks the execution stack size computation against hand
// computed values: the peak number of stack elements, including the initial
// witness elements. This is the property that feeds the total stack size limit
// of both segwit contexts. It is not compared against rust-miniscript, whose
// value is a looser, order-dependent estimate.
func TestExecStack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		miniscript string
		ctx        Context
		exec       int
	}{{
		// The signature, then the pushed public key.
		miniscript: "pk(A)",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// OP_CHECKMULTISIG has the dummy zero, the two signatures, <k>,
		// all n keys and <n> on the stack.
		miniscript: "multi(2,A,B,C)",
		ctx:        P2WSH,
		exec:       8,
	}, {
		// Dissatisfying X starts from two elements (the signature for B
		// and the empty one for A) and pushes the key of A on top.
		miniscript: "or_d(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       3,
	}, {
		// The Z branch starts from the four elements of both witnesses,
		// of which the dissatisfaction of X is consumed before the
		// multi pushes its five elements on top of the other three.
		miniscript: "or_d(pk(A),multi(2,B,C,D))",
		ctx:        P2WSH,
		exec:       8,
	}, {
		// Both signatures, then the key of A on top. The result of A
		// then stays on the stack while B pushes its key.
		miniscript: "and_b(pk(A),s:pk(B))",
		ctx:        P2WSH,
		exec:       3,
	}, {
		// The signature and the branch selector, which OP_IF consumes
		// before the key is pushed.
		miniscript: "or_i(pk(A),pk(B))",
		ctx:        P2WSH,
		exec:       2,
	}, {
		// Three witness elements and the first key.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2WSH,
		exec:       4,
	}, {
		// Three witness elements and the first key.
		miniscript: "multi_a(2,A,B,C)",
		ctx:        P2TR,
		exec:       4,
	}, {
		// Two signatures and the first key.
		miniscript: "and_v(v:pk(A),pk(B))",
		ctx:        P2TR,
		exec:       3,
	}, {
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		ctx:        P2TR,
		exec:       4,
	}, {
		// Core's test vector for the same expression: two witness
		// elements and the first key.
		miniscript: "and_v(v:multi_a(2,A,B),after(1231488000))",
		ctx:        P2TR,
		exec:       3,
	}}

	for _, tc := range cases {
		node, err := Parse(tc.miniscript, tc.ctx)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)
		require.Equalf(t, tc.exec, node.maxExecStackSize(), "exec "+
			"stack size for %s", tc.miniscript)
	}
}

// tapRedeem builds a taproot output whose only script-path leaf is the given
// P2TR miniscript, then spends it via the script path using a satisfaction
// generated from the miniscript, and runs the script engine. It returns any
// error from sanity checking, satisfaction or execution.
func tapRedeem(t *testing.T, miniscript string,
	lookupVar func(string) ([]byte, error), sequence uint32,
	signable map[string]*btcec.PrivateKey, preimage PreimageFunc) error {

	t.Helper()

	node, err := Parse(miniscript, P2TR)
	if err != nil {
		return err
	}

	return tapRedeemNode(t, node, lookupVar, sequence, signable, preimage)
}

// tapRedeemNode is tapRedeem for an expression that is parsed already.
func tapRedeemNode(t *testing.T, node *AST,
	lookupVar func(string) ([]byte, error), sequence uint32,
	signable map[string]*btcec.PrivateKey, preimage PreimageFunc) error {

	t.Helper()

	// Resolve the keys only after the symbolic expression passes sanity
	// checks, so failures remain attributable to the appropriate stage.
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
				lockTime, transaction.Version,
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
		// A sortedmulti_a is a multi_a whose keys are sorted, so its
		// satisfaction has to follow the sorted order to execute.
		name:       "2-of-3 sortedmulti_a",
		miniscript: "sortedmulti_a(2,A,B,C)",
		signers:    []string{"A", "B"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "2-of-3 sortedmulti_a other signers",
		miniscript: "sortedmulti_a(2,A,B,C)",
		signers:    []string{"B", "C"},
		preimage:   noPreimage,
		valid:      true,
	}, {
		name:       "2-of-3 sortedmulti_a insufficient",
		miniscript: "sortedmulti_a(2,A,B,C)",
		signers:    []string{"C"},
		preimage:   noPreimage,
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

// TestAllowDuplicateKeys checks that a whole multi, multi_a or sortedmulti_a
// may be allowed to repeat a key, as Core allows for the multi() and
// sortedmulti() descriptors and for a tr() leaf of its own, and that the result
// spends: a signature with the repeated key counts once per occurrence. Any
// other expression keeps rejecting repeated keys.
func TestAllowDuplicateKeys(t *testing.T) {
	t.Parallel()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	xOnly := schnorr.SerializePubKey(privKey.PubKey())
	other, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	otherXOnly := schnorr.SerializePubKey(other.PubKey())

	lookupVar := func(id string) ([]byte, error) {
		switch id {
		case "A", "A2":
			return xOnly, nil

		case "B":
			return otherXOnly, nil
		}
		return nil, nil
	}
	signable := map[string]*btcec.PrivateKey{
		hex.EncodeToString(xOnly): privKey,
	}
	noPreimage := func(string, []byte) ([]byte, bool) { return nil, false }

	for _, expr := range []string{
		"multi_a(2,A,A,B)",
		"sortedmulti_a(2,A,B,A)",
		// Two identifiers that resolve to the same key.
		"multi_a(2,A,A2,B)",
	} {

		node, err := ParseInsane(expr, P2TR)
		require.NoError(t, err)

		if expr != "multi_a(2,A,A2,B)" {
			require.ErrorContains(t, node.IsSane(), "duplicate key")
		}

		require.NoError(t, node.AllowDuplicateKeys())
		require.NoError(t, node.IsSane())

		// The clone keeps the setting.
		require.NoError(t, tapRedeemNode(
			t, node.Clone(), lookupVar, 0, signable, noPreimage,
		), expr)
	}

	// The same holds for a whole multi in P2WSH, which is what the
	// multi() and sortedmulti() descriptors compile to. A signature for
	// the repeated key matches both of its occurrences.
	compressed := privKey.PubKey().SerializeCompressed()
	otherCompressed := other.PubKey().SerializeCompressed()
	lookupCompressed := func(id string) ([]byte, error) {
		switch id {
		case "A", "A2":
			return compressed, nil

		case "B":
			return otherCompressed, nil
		}
		return nil, nil
	}
	sign := func(pubKey, hash []byte) ([]byte, bool) {
		if !bytes.Equal(pubKey, compressed) {
			return nil, false
		}
		return ecdsa.Sign(privKey, hash).Serialize(), true
	}
	for _, expr := range []string{
		"multi(2,A,A,B)",
		"multi(1,A,A)",
		"multi(2,A,A2,B)",
	} {

		node, err := ParseInsane(expr, P2WSH)
		require.NoError(t, err)
		require.NoError(t, node.AllowDuplicateKeys())
		require.NoError(t, testRedeemNode(
			t, node, lookupCompressed, 0, sign, noPreimage,
		), expr)
	}

	// A repeated key is still rejected everywhere else.
	node, err := ParseInsane("and_v(v:pk(B),multi_a(1,A,A))", P2TR)
	require.NoError(t, err)
	require.ErrorContains(
		t, node.AllowDuplicateKeys(),
		"duplicate keys can only be allowed in a whole multi, multi_a "+
			"or sortedmulti_a",
	)
	require.ErrorContains(t, node.IsSane(), "duplicate key")

	node, err = ParseInsane("multi_a(2,A,A2,B)", P2TR)
	require.NoError(t, err)
	require.ErrorContains(
		t, node.ApplyVars(lookupVar), "duplicate key found",
	)
}

// TestSortedMultiA checks that sortedmulti_a is a multi_a whose keys are sorted
// by their serialization once they are known (BIP387): the script it builds has
// to be the one multi_a builds for the sorted key order, no matter which order
// the descriptor writes them in.
func TestSortedMultiA(t *testing.T) {
	t.Parallel()

	// Three x-only keys, deliberately not in sorted order in the
	// expressions below.
	keys := map[string][]byte{
		"A": bytes.Repeat([]byte{0xcc}, xOnlyPubKeyLen),
		"B": bytes.Repeat([]byte{0xaa}, xOnlyPubKeyLen),
		"C": bytes.Repeat([]byte{0xbb}, xOnlyPubKeyLen),
	}
	lookupVar := func(id string) ([]byte, error) {
		return keys[id], nil
	}

	script := func(expr string) []byte {
		node, err := Parse(expr, P2TR)
		require.NoError(t, err)
		require.NoError(t, node.ApplyVars(lookupVar))

		built, err := node.Script()
		require.NoError(t, err)

		return built
	}

	// B, C, A is the sorted order of the three keys.
	sorted := script("multi_a(2,B,C,A)")
	require.Equal(t, sorted, script("sortedmulti_a(2,A,B,C)"))
	require.Equal(t, sorted, script("sortedmulti_a(2,C,B,A)"))
	require.Equal(t, sorted, script("sortedmulti_a(2,B,C,A)"))

	// The unsorted spelling of the same keys builds a different script,
	// which is what makes the two fragments distinguishable at all.
	require.NotEqual(t, sorted, script("multi_a(2,A,B,C)"))

	// The sorting only reorders the keys, so everything the analysis
	// computes stays the same as for the equivalent multi_a.
	sortedAST, err := Parse("sortedmulti_a(2,A,B,C)", P2TR)
	require.NoError(t, err)
	plainAST, err := Parse("multi_a(2,A,B,C)", P2TR)
	require.NoError(t, err)

	require.Equal(t, plainAST.ScriptLen(), sortedAST.ScriptLen())
	require.Equal(t, plainAST.stackSize, sortedAST.stackSize)
	require.Equal(t, plainAST.satSize, sortedAST.satSize)
	require.Equal(t, plainAST.execStack, sortedAST.execStack)
	require.Equal(t, plainAST.props, sortedAST.props)
	require.Equal(t, plainAST.Keys(), sortedAST.Keys())

	// sortedmulti_a only exists from taproot on, like multi_a.
	for _, ctx := range []Context{P2WSH, Legacy} {
		_, err := Parse("sortedmulti_a(2,A,B,C)", ctx)
		require.ErrorContains(
			t, err,
			"sortedmulti_a is not allowed in the "+
				ctx.String(),
		)
	}

	// BIP387 defines sortedmulti_a as a whole tr() leaf, not as a
	// miniscript fragment, so it can be neither wrapped nor nested, as in
	// Core.
	for _, expr := range []string{
		"n:sortedmulti_a(1,A,B)",
		"and_v(v:pk(C),sortedmulti_a(1,A,B))",
		"and_v(v:sortedmulti_a(2,A,B,C),older(2))",
		"thresh(1,pk(C),s:sortedmulti_a(1,A,B))",
	} {

		_, err := ParseInsane(expr, P2TR)
		require.ErrorContainsf(
			t, err, "sortedmulti_a", "expression %s", expr,
		)
	}

	// The key count limit of multi_a applies as well.
	var many strings.Builder
	many.WriteString("sortedmulti_a(1")
	for i := range checkSigAddMaxKeys + 1 {
		fmt.Fprintf(&many, ",key%d", i)
	}
	many.WriteByte(')')

	_, err = Parse(many.String(), P2TR)
	require.ErrorContains(t, err, "cannot exceed 999")
}

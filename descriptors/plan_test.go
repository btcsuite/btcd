package descriptors

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// testTapLeafSig is a 64-byte dummy Schnorr signature used in the plan tests.
var testTapLeafSig = []byte(
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
)

// testEcdsaSigHex is a valid DER-encoded ECDSA signature (with its sighash
// byte) used in the P2WSH plan tests.
const testEcdsaSigHex = "3045022100e621a7686d51fb23e761adff4367881a6fb16b" +
	"c5635ff34eea39afdaf033e4d702207998512f52bd3dae100951a6df9e66bcb78" +
	"c194dcaa3c7fd2451180b5cc94d4e01"

// TestPlanAt exercises the spending-plan construction and satisfaction for
// taproot key-path, taproot script-path and P2WSH (native and P2SH-wrapped)
// descriptors, against the descriptors-go reference values.
func TestPlanAt(t *testing.T) {
	t.Parallel()

	descriptorTr, err := NewDescriptor(testTr)
	require.NoError(t, err)

	// The definite key strings the lookups should be called with, at
	// multipath index 0 and derivation index 0.
	internalDef := "[e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3sUo5nUMPEv6" +
		"FZ81GHn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdYZwGLECtg" +
		"QruG4FkZMh566RkfUT2pbzsEg/0/0"
	leafDef := "[3c157b79/48'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mSuWgf" +
		"mPdBduXWzSP91Y3GxKWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5kSbi9" +
		"yJDjuJUDzQSWUh3ijzXP/0/0"
	leafHash := "fc5460a80d4b2477db9612cb453da10d33c8dffa569c7c40efe94e" +
		"0591451120"

	// A taproot leaf script spend fails without a relative locktime, and
	// with one that is too small.
	t.Run("taproot leaf script fail", func(t *testing.T) {
		t.Parallel()

		leafSig := func(pk, lh string) (uint32, bool) {
			return 64, true
		}

		_, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: leafSig,
		})
		require.Error(t, err)

		tooSmall := uint32(65535 - 1)
		_, err = descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: leafSig,
			RelativeLocktime:       &tooSmall,
		})
		require.Error(t, err)
	})

	t.Run("taproot leaf script OK", func(t *testing.T) {
		t.Parallel()

		locktimeOK := uint32(65535)
		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: func(pk, lh string) (uint32,
				bool) {

				require.Equal(t, leafDef, pk)
				require.Equal(t, leafHash, lh)
				return 64, true
			},
			RelativeLocktime: &locktimeOK,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(144), plan.SatisfactionWeight())
		require.Equal(t, uint64(1), plan.ScriptSigSize())
		require.Equal(t, uint64(140), plan.WitnessSize())

		result, err := plan.Satisfy(&Satisfier{
			LookupTapLeafScriptSig: func(pk, lh string) ([]byte,
				bool) {

				return testTapLeafSig, true
			},
		})
		require.NoError(t, err)

		script, err := hex.DecodeString(
			"200b44e43e2f276697d23c2248f80bb09e84f702ddae399d194f" +
				"5132f472bf8713ad03ffff00b2",
		)
		require.NoError(t, err)
		controlBlock, err := hex.DecodeString(
			"c126547ceb5352bd238ca7e1da004e9d6625baf3324feda4ead6" +
				"9436042a535104",
		)
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness: [][]byte{
				testTapLeafSig, script, controlBlock,
			},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("taproot key path spend OK", func(t *testing.T) {
		t.Parallel()

		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapKeySpendSig: func(pk string) (uint32, bool) {
				require.Equal(t, internalDef, pk)
				return 64, true
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint64(70), plan.SatisfactionWeight())

		result, err := plan.Satisfy(&Satisfier{
			LookupTapKeySpendSig: func() ([]byte, bool) {
				return testTapLeafSig, true
			},
		})
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{testTapLeafSig},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("wsh OK", func(t *testing.T) {
		t.Parallel()

		descriptor, err := NewDescriptor("wsh(pk(" + testXpub1 + "))")
		require.NoError(t, err)

		// testXpub1 is the same key as the taproot internal key, so its
		// definite string at (0, 0) is internalDef.
		plan, err := descriptor.PlanAt(0, 0, Assets{
			LookupEcdsaSig: func(pk string) bool {
				require.Equal(t, internalDef, pk)
				return true
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint64(78), plan.SatisfactionWeight())
		require.Equal(t, uint64(1), plan.ScriptSigSize())
		require.Equal(t, uint64(74), plan.WitnessSize())

		sig, err := hex.DecodeString(testEcdsaSigHex)
		require.NoError(t, err)
		result, err := plan.Satisfy(&Satisfier{
			LookupEcdsaSig: func(pk string) ([]byte, bool) {
				return sig, true
			},
		})
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{sig},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("wsh-sh OK", func(t *testing.T) {
		t.Parallel()

		descriptor, err := NewDescriptor(
			"sh(wsh(pk(" + testXpub1 + ")))",
		)
		require.NoError(t, err)

		plan, err := descriptor.PlanAt(0, 0, Assets{
			LookupEcdsaSig: func(pk string) bool {
				return true
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint64(214), plan.SatisfactionWeight())
		require.Equal(t, uint64(35), plan.ScriptSigSize())
		require.Equal(t, uint64(74), plan.WitnessSize())

		sig, err := hex.DecodeString(testEcdsaSigHex)
		require.NoError(t, err)
		result, err := plan.Satisfy(&Satisfier{
			LookupEcdsaSig: func(pk string) ([]byte, bool) {
				return sig, true
			},
		})
		require.NoError(t, err)

		scriptSig, err := hex.DecodeString(
			"220020d5c86b71799a3e4f4db05698009efa8eed80a86ea47b5c" +
				"aebc47b01d5384b2f1",
		)
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{sig},
			ScriptSig: scriptSig,
		}, result)
	})

	t.Run("plan satisfy fail", func(t *testing.T) {
		t.Parallel()

		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapKeySpendSig: func(pk string) (uint32, bool) {
				return 64, true
			},
		})
		require.NoError(t, err)

		_, err = plan.Satisfy(&Satisfier{})
		require.EqualError(t, err, "could not satisfy")
	})
}

// TestPlanTapscriptTreeSpend plans and completes a script-path spend for each
// leaf of an unbalanced taproot script tree, then verifies the resulting
// witness (leaf script and control block) with the script engine. This
// exercises control-block construction at multiple tree depths end to end.
func TestPlanTapscriptTreeSpend(t *testing.T) {
	t.Parallel()

	// Generate an internal key and three leaf keys, keyed by name.
	priv := make(map[string]*btcec.PrivateKey)
	xhex := make(map[string]string)
	for _, name := range []string{"I", "A", "B", "C"} {
		p, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		priv[name] = p
		xhex[name] = hex.EncodeToString(
			schnorr.SerializePubKey(p.PubKey()),
		)
	}

	// An unbalanced tree: leaf A at depth 1, leaves B and C at depth 2.
	desc := fmt.Sprintf("tr(%s,{pk(%s),{pk(%s),pk(%s)}})",
		xhex["I"], xhex["A"], xhex["B"], xhex["C"])
	d, err := NewDescriptor(desc)
	require.NoError(t, err)

	outputKey, err := d.taprootOutputKey(d.root, 0, 0)
	require.NoError(t, err)
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	const amount = int64(100000)

	for _, leaf := range []string{"A", "B", "C"} {
		leaf := leaf
		t.Run("leaf "+leaf, func(t *testing.T) {
			t.Parallel()

			leafHex := xhex[leaf]

			// Only this leaf's signature is available, so the plan
			// must choose this leaf's script path.
			plan, err := d.PlanAt(0, 0, Assets{
				LookupTapLeafScriptSig: func(pk,
					lh string) (uint32, bool) {

					return 64, pk == leafHex
				},
			})
			require.NoError(t, err)

			// Build a transaction spending the taproot output.
			prevFetcher := txscript.NewCannedPrevOutputFetcher(
				pkScript, amount,
			)
			spendTx := wire.NewMsgTx(2)
			spendTx.AddTxIn(wire.NewTxIn(
				&wire.OutPoint{}, nil, nil,
			))
			spendTx.AddTxOut(&wire.TxOut{
				Value: amount - 500, PkScript: pkScript,
			})
			sigHashes := txscript.NewTxSigHashes(
				spendTx, prevFetcher,
			)

			// Satisfy the plan by signing the chosen leaf.
			result, err := plan.Satisfy(&Satisfier{
				LookupTapLeafScriptSig: func(pk,
					lh string) ([]byte, bool) {

					if pk != leafHex {
						return nil, false
					}
					return signLeaf(
						t, spendTx, sigHashes, amount,
						pkScript, leafHex, priv[leaf],
					), true
				},
			})
			require.NoError(t, err)
			require.Empty(t, result.ScriptSig)

			// The witness must satisfy the taproot output under the
			// standard consensus rules.
			spendTx.TxIn[0].Witness = result.Witness
			engine, err := txscript.NewEngine(
				pkScript, spendTx, 0,
				txscript.StandardVerifyFlags, nil, sigHashes,
				amount, prevFetcher,
			)
			require.NoError(t, err)
			require.NoError(t, engine.Execute())
		})
	}
}

// signLeaf produces a BIP340 signature over the tapscript leaf "pk(<key>)" for
// the given signer, used to complete a plan in the taproot tree spend test.
func signLeaf(t *testing.T, tx *wire.MsgTx, sigHashes *txscript.TxSigHashes,
	amount int64, pkScript []byte, keyHex string,
	signer *btcec.PrivateKey) []byte {

	keyBytes, err := hex.DecodeString(keyHex)
	require.NoError(t, err)

	leafScript, err := txscript.NewScriptBuilder().AddData(keyBytes).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)

	sig, err := txscript.RawTxInTapscriptSignature(
		tx, sigHashes, 0, amount, pkScript,
		txscript.NewBaseTapLeaf(leafScript), txscript.SigHashDefault,
		signer,
	)
	require.NoError(t, err)
	return sig
}

// TestPlanWeights checks the weight accounting of a plan: the satisfaction
// weight is the witness size plus four times the scriptSig size, the getters
// return the stored sizes, and Satisfy dispatches to the plan's closure.
func TestPlanWeights(t *testing.T) {
	t.Parallel()

	called := false
	plan := &Plan{
		witnessSize:   100,
		scriptSigSize: 35,
		satisfy: func(*Satisfier) (*SatisfyResult, error) {
			called = true
			return &SatisfyResult{ScriptSig: []byte{0x01}}, nil
		},
	}

	require.Equal(t, uint64(100), plan.WitnessSize())
	require.Equal(t, uint64(35), plan.ScriptSigSize())
	require.Equal(t, uint64(100+35*4), plan.SatisfactionWeight())

	result, err := plan.Satisfy(&Satisfier{})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []byte{0x01}, result.ScriptSig)
}

// TestWitnessSerializedSize checks the serialized size of a witness stack: the
// element-count var-int plus each element's length var-int and its bytes.
func TestWitnessSerializedSize(t *testing.T) {
	t.Parallel()

	// Empty stack: only the count var-int (one byte for zero).
	require.Equal(t, uint64(1), witnessSerializedSize([][]byte{}))

	// One 64-byte element: count(1) + len(1) + 64.
	require.Equal(t, uint64(66), witnessSerializedSize(
		[][]byte{make([]byte, 64)},
	))

	// Two elements: count(1) + (1 + 72) + (1 + 33).
	require.Equal(t, uint64(108), witnessSerializedSize(
		[][]byte{make([]byte, 72), make([]byte, 33)},
	))

	// A 253-byte element needs a three-byte length var-int: count(1) + 3 +
	// 253.
	require.Equal(t, uint64(257), witnessSerializedSize(
		[][]byte{make([]byte, 253)},
	))
}

// TestRelativeImplied checks the BIP68 relative-locktime "implied by" test: a
// missing maximum is never satisfied, the block and time units must match, and
// the required value must not exceed the maximum.
func TestRelativeImplied(t *testing.T) {
	t.Parallel()

	ptr := func(v uint32) *uint32 { return &v }
	seconds := uint32(1) << 22

	require.False(t, relativeImplied(10, nil))
	require.True(t, relativeImplied(10, ptr(20)))
	require.True(t, relativeImplied(20, ptr(20)))
	require.False(t, relativeImplied(30, ptr(20)))
	require.False(t, relativeImplied(10, ptr(seconds|20)))
	require.True(t, relativeImplied(seconds|10, ptr(seconds|20)))
}

// TestAbsoluteImplied checks the BIP65 absolute-locktime "implied by" test: a
// missing maximum is never satisfied, the height and time kinds must match, and
// the required value must not exceed the maximum.
func TestAbsoluteImplied(t *testing.T) {
	t.Parallel()

	ptr := func(v uint32) *uint32 { return &v }
	const threshold = uint32(500000000)

	require.False(t, absoluteImplied(100, nil))
	require.True(t, absoluteImplied(100, ptr(200)))
	require.False(t, absoluteImplied(300, ptr(200)))
	require.False(t, absoluteImplied(100, ptr(threshold+5)))
	require.True(t, absoluteImplied(threshold+100, ptr(threshold+200)))
}

// TestBranchHash checks that the taproot branch hash matches the reference
// txscript implementation and is independent of the argument order (the two
// child hashes are sorted before hashing).
func TestBranchHash(t *testing.T) {
	t.Parallel()

	leafA := txscript.NewBaseTapLeaf([]byte{txscript.OP_1})
	leafB := txscript.NewBaseTapLeaf([]byte{txscript.OP_2})
	hashA, hashB := leafA.TapHash(), leafB.TapHash()

	want := txscript.NewTapBranch(leafA, leafB).TapHash()
	require.Equal(t, want, branchHash(hashA, hashB))

	// Swapping the two children yields the same branch hash.
	require.Equal(t, branchHash(hashA, hashB), branchHash(hashB, hashA))
}

// TestPushScriptAndAll checks the script-push helpers: pushScript pushes a
// single element, pushAll pushes several in order.
func TestPushScriptAndAll(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte{0xab}, 34)
	script, err := pushScript(data)
	require.NoError(t, err)
	require.Equal(t, append([]byte{0x22}, data...), script)

	first := bytes.Repeat([]byte{0x01}, 33)
	second := bytes.Repeat([]byte{0x02}, 20)
	all, err := pushAll([][]byte{first, second})
	require.NoError(t, err)

	want := append([]byte{0x21}, first...)
	want = append(want, 0x14)
	want = append(want, second...)
	require.Equal(t, want, all)
}

// TestDescKeyDefiniteString checks that a key's path is resolved at a given
// multipath and derivation index: the multipath element is replaced by its
// selected value and the wildcard by the derivation index, while the origin and
// fixed steps are kept.
func TestDescKeyDefiniteString(t *testing.T) {
	t.Parallel()

	origin := "[e81a5744/48'/0'/0'/2']"
	tests := []struct {
		name string
		raw  string
		mp   uint32
		idx  uint32
		want string
	}{{
		name: "no path",
		raw:  basicTestXpub,
		want: basicTestXpub,
	}, {
		name: "wildcard",
		raw:  basicTestXpub + "/*",
		idx:  5,
		want: basicTestXpub + "/5",
	}, {
		name: "fixed path",
		raw:  basicTestXpub + "/3/7",
		want: basicTestXpub + "/3/7",
	}, {
		name: "origin multipath wildcard",
		raw:  origin + basicTestXpub + "/<0;1>/*",
		mp:   1,
		idx:  4,
		want: origin + basicTestXpub + "/1/4",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			k, err := parseDescKey(tc.raw)
			require.NoError(t, err)
			require.Equal(
				t, tc.want,
				k.definiteString(tc.mp, tc.idx),
			)
		})
	}
}

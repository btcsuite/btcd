package descriptors

import (
	"encoding/hex"
	"fmt"
	"math/bits"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// checkDescriptorSpend tests a bounded family with an independent availability
// rule: k of three keys, or (in the two-leaf taproot case) a fourth key after
// two blocks. It checks planning refusal as well as cryptographically valid
// spends.
func checkDescriptorSpend(t *testing.T, kind, threshold, mask byte,
	sequence uint32) {

	t.Helper()
	kind, threshold = kind%5, 1+threshold%3
	tap := kind >= 3

	// Private scalars are deterministic and distinct, including an internal
	// key for which no key-path signature is offered. Availability is fixed
	// before any production parsing, analysis or planning takes place.
	keys := make([]string, 5)
	private := make(map[string]*btcec.PrivateKey)
	available := make(map[string]bool)
	for i := range keys {
		priv, pub := btcec.PrivKeyFromBytes([]byte{byte(i + 1)})
		serialized := pub.SerializeCompressed()
		if tap {
			serialized = schnorr.SerializePubKey(pub)
		}
		keys[i] = hex.EncodeToString(serialized)
		private[keys[i]] = priv
		available[keys[i]] = mask&(1<<i) != 0
	}
	want := bits.OnesCount8(mask&7) >= int(threshold)
	if kind == 4 {
		want = want || (mask&8 != 0 && sequence&(1<<31|1<<22) == 0 && sequence&0xffff >= 2)
	}
	fragment := "multi"
	if tap {
		fragment = "multi_a"
	}
	primary := fmt.Sprintf("%s(%d,%s)", fragment, threshold, strings.Join(
		keys[:3], ",",
	))
	fallback := "and_v(v:pk(" + keys[3] + "),older(2))"
	expression := []string{
		"wsh(" + primary + ")", "sh(wsh(" + primary + "))", "sh(" + primary + ")",
		"tr(" + keys[4] + "," + primary + ")",
		"tr(" + keys[4] + ",{" + primary + "," + fallback + "})",
	}[kind]
	d, err := NewDescriptor(expression)
	require.NoError(t, err)
	version := int32(2)
	plan, err := d.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(key string) bool { return available[key] },
		LookupTapLeafScriptSig: func(key, _ string) (uint32, bool) {
			return 64, available[key]
		},
		TxVersion: &version, TxInputSequence: &sequence,
	})
	if !want {
		require.Error(
			t, err, "%s mask=%d sequence=%d", expression, mask,
			sequence,
		)
		return
	}
	require.NoError(t, err, expression)

	// Derive the actual output and transaction, not an output synthesized
	// from the planner's chosen witness. This catches missing or incorrect
	// redeem scripts, witness scripts, leaf commitments and control blocks.
	encoded, err := d.AddressAt(&chaincfg.RegressionNetParams, 0, 0)
	require.NoError(t, err)
	addr, err := address.DecodeAddress(
		encoded, &chaincfg.RegressionNetParams,
	)
	require.NoError(t, err)
	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)
	const amount = int64(10000)
	tx := wire.NewMsgTx(version)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, nil))
	tx.TxIn[0].Sequence = sequence
	tx.AddTxOut(wire.NewTxOut(amount-1000, []byte{txscript.OP_RETURN}))
	prevouts := txscript.NewCannedPrevOutputFetcher(pkScript, amount)
	hashes := txscript.NewTxSigHashes(tx, prevouts)
	var script []byte
	leaves := make(map[string]txscript.TapLeaf)
	if tap {
		// The signer knows the source leaves; it refuses unknown hashes
		// instead of trusting a hash requested by the plan blindly.
		for _, source := range []string{primary, fallback} {
			node, err := miniscript.Parse(source, miniscript.P2TR)
			require.NoError(t, err)
			require.NoError(
				t, node.ApplyVars(func(string) ([]byte, error) {
					return nil, nil
				}),
			)
			compiled, err := node.Script()
			require.NoError(t, err)
			leaf := txscript.NewBaseTapLeaf(compiled)
			hash := leaf.TapHash()
			leaves[hex.EncodeToString(hash[:])] = leaf
		}
	} else {
		script, err = d.ScriptCodeAt(0, 0)
		require.NoError(t, err)
	}
	signer := &Satisfier{
		LookupEcdsaSig: func(key string) ([]byte, bool) {
			if !available[key] {
				return nil, false
			}
			var signature []byte
			var err error
			if kind == 2 {
				signature, err = txscript.RawTxInSignature(
					tx, 0, script, txscript.SigHashAll,
					private[key],
				)
			} else {
				signature, err = txscript.RawTxInWitnessSignature(tx, hashes, 0, amount, script, txscript.SigHashAll, private[key])
			}
			require.NoError(t, err)
			return signature, true
		},
		LookupTapLeafScriptSig: func(key, hash string) ([]byte, bool) {
			leaf, known := leaves[hash]
			require.True(t, known, "unknown leaf %s", hash)
			if !available[key] {
				return nil, false
			}
			signature, err := txscript.RawTxInTapscriptSignature(
				tx, hashes, 0, amount, pkScript, leaf,
				txscript.SigHashDefault, private[key],
			)
			require.NoError(t, err)
			return signature, true
		},
	}

	// Every branch is signature-bound, so an empty completion must fail.
	// Retrying with the promised data must still succeed on the same plan.
	_, err = plan.Satisfy(&Satisfier{})
	require.Error(t, err)
	result, err := plan.Satisfy(signer)
	require.NoError(t, err)
	tx.TxIn[0].Witness = result.Witness
	tx.TxIn[0].SignatureScript = result.ScriptSig
	scriptSize := len(result.ScriptSig) +
		wire.VarIntSerializeSize(uint64(len(result.ScriptSig)))
	require.LessOrEqual(t, uint64(scriptSize), plan.ScriptSigSize())
	if len(result.Witness) != 0 {
		require.LessOrEqual(
			t, uint64(tx.TxIn[0].Witness.SerializeSize()),
			plan.WitnessSize(),
		)
	} else {
		require.Zero(t, plan.WitnessSize())
	}
	engine, err := txscript.NewEngine(
		pkScript, tx, 0, txscript.StandardVerifyFlags, nil, hashes,
		amount, prevouts,
	)
	require.NoError(t, err)
	require.NoError(t, engine.Execute(), expression)
}

// TestDescriptorSpendingProperties exhausts the available signer subsets for
// all wrappers and thresholds, on both sides of the recovery-lock boundary.
func TestDescriptorSpendingProperties(t *testing.T) {
	t.Parallel()
	for kind := range byte(5) {
		for threshold := range byte(3) {
			for mask := range byte(16) {
				for _, sequence := range []uint32{1, 2} {
					checkDescriptorSpend(
						t, kind, threshold, mask,
						sequence,
					)
				}
			}
		}
	}
}

// FuzzDescriptorSpending mutates policy choices and available assets while
// retaining valid syntax, then signs and executes every expected-success plan.
func FuzzDescriptorSpending(f *testing.F) {
	for kind := range byte(5) {
		f.Add(kind, byte(1), byte(15), uint32(2))
		f.Add(kind, byte(1), byte(0), uint32(1))
	}
	f.Fuzz(func(t *testing.T, kind, threshold, mask byte, sequence uint32) {
		checkDescriptorSpend(t, kind, threshold, mask, sequence)
	})
}

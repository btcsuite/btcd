package descriptors_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/descriptors"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// spendingVectors is the implementation-independent fixture file format.
type spendingVectors struct {
	Version     int              `json:"version"`
	Description string           `json:"description"`
	Cases       []spendingVector `json:"cases"`
}

// spendingVector describes a plan and independent attempts to complete it.
type spendingVector struct {
	ID              string             `json:"id"`
	Descriptor      string             `json:"descriptor"`
	MultipathIndex  uint32             `json:"multipath_index"`
	DerivationIndex uint32             `json:"derivation_index"`
	Tx              vectorTransaction  `json:"tx"`
	Assets          vectorAssets       `json:"assets"`
	ExpectedPlan    vectorPlan         `json:"expected_plan"`
	ScriptPubKey    string             `json:"script_pubkey"`
	Completions     []vectorCompletion `json:"completions"`
	Transaction     *vectorSpend       `json:"transaction,omitempty"`
}

// vectorSpend provides the unsigned transaction and every input's previous
// output, in input order, for independent signature and script verification.
type vectorSpend struct {
	UnsignedTx string          `json:"unsigned_tx"`
	InputIndex int             `json:"input_index"`
	Prevouts   []vectorPrevout `json:"prevouts"`
}

// vectorPrevout specifies a spent output's amount in satoshis and script bytes.
type vectorPrevout struct {
	Value        int64  `json:"value"`
	ScriptPubKey string `json:"script_pubkey"`
}

// vectorTransaction carries optional transaction fields for locktime checks.
type vectorTransaction struct {
	Version  *int32  `json:"version"`
	Sequence *uint32 `json:"sequence"`
	LockTime *uint32 `json:"lock_time"`
}

// vectorAssets contains availability only, without concrete signatures.
type vectorAssets struct {
	ECDSA     []string         `json:"ecdsa"`
	TapKey    []vectorTapAsset `json:"tap_key"`
	TapLeaf   []vectorTapAsset `json:"tap_leaf"`
	Preimages []vectorPreimage `json:"preimages"`
}

// vectorTapAsset identifies a key-path or leaf-specific signature and its size.
type vectorTapAsset struct {
	Key      string `json:"key"`
	LeafHash string `json:"leaf_hash,omitempty"`
	Size     uint32 `json:"size"`
}

// vectorPreimage identifies a hash algorithm and a digest in script byte order.
type vectorPreimage struct {
	Function string `json:"function"`
	Hash     string `json:"hash"`
	Preimage string `json:"preimage,omitempty"`
}

// vectorPlan specifies complete serialized input satisfaction sizes.
type vectorPlan struct {
	Error         string `json:"error,omitempty"`
	WitnessSize   uint64 `json:"witness_size,omitempty"`
	ScriptSigSize uint64 `json:"script_sig_size,omitempty"`
	Weight        uint64 `json:"weight,omitempty"`
}

// vectorCompletion supplies concrete data independently of planning assets.
type vectorCompletion struct {
	ID       string       `json:"id"`
	Data     vectorData   `json:"data"`
	Expected vectorResult `json:"expected"`
	Verify   bool         `json:"verify,omitempty"`
}

// vectorData is the concrete signature and preimage lookup table.
type vectorData struct {
	ECDSA     map[string]string    `json:"ecdsa"`
	TapKey    map[string]string    `json:"tap_key"`
	TapLeaf   []vectorTapSignature `json:"tap_leaf"`
	Preimages []vectorPreimage     `json:"preimages"`
}

// vectorTapSignature associates signature bytes with one exact Taproot leaf.
type vectorTapSignature struct {
	Key       string `json:"key"`
	LeafHash  string `json:"leaf_hash"`
	Signature string `json:"signature"`
}

// vectorResult contains transaction-ready bytes or a failure stage.
type vectorResult struct {
	Error     string   `json:"error,omitempty"`
	Witness   []string `json:"witness,omitempty"`
	ScriptSig string   `json:"script_sig,omitempty"`
	Valid     *bool    `json:"valid,omitempty"`
}

// verify executes the actual completed input under standard script flags. It
// never substitutes a fixture witness for the library's returned satisfaction.
func (v vectorSpend) verify(t *testing.T,
	result *descriptors.SatisfyResult) bool {

	t.Helper()

	// Decode exactly one transaction so trailing fixture bytes cannot be
	// mistaken for part of a valid verification-negative test case.
	var tx wire.MsgTx
	reader := bytes.NewReader(vectorBytes(t, v.UnsignedTx))
	require.NoError(t, tx.Deserialize(reader))
	require.Zero(t, reader.Len())

	// Taproot sighashes commit to previous outputs across all inputs.
	// Require complete, ordered prevout data before constructing the
	// sighash cache.
	require.Len(t, v.Prevouts, len(tx.TxIn))
	require.GreaterOrEqual(t, v.InputIndex, 0)
	require.Less(t, v.InputIndex, len(tx.TxIn))
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for i, prev := range v.Prevouts {
		fetcher.AddPrevOut(tx.TxIn[i].PreviousOutPoint, &wire.TxOut{
			Value:    prev.Value,
			PkScript: vectorBytes(t, prev.ScriptPubKey),
		})
	}

	// Insert the library's actual result. Using the expected fixture stack
	// here would verify the vector without verifying the implementation.
	tx.TxIn[v.InputIndex].SignatureScript = result.ScriptSig
	tx.TxIn[v.InputIndex].Witness = result.Witness
	previous := v.Prevouts[v.InputIndex]
	engine, err := txscript.NewEngine(
		vectorBytes(t, previous.ScriptPubKey), &tx, v.InputIndex,
		txscript.StandardVerifyFlags, nil,
		txscript.NewTxSigHashes(&tx, fetcher), previous.Value, fetcher,
	)

	// Both engine construction and execution can reject a spend. Either
	// rejection means invalid Script, distinct from a fixture decoding
	// error.
	return err == nil && engine.Execute() == nil
}

// checkVectorCompletion repeats each completion and checks byte ownership as
// well as exact stacks, errors and optional transaction verification results.
func checkVectorCompletion(t *testing.T, plan *descriptors.Plan,
	v spendingVector, c vectorCompletion) {

	t.Helper()

	// Repeating each attempt after damaging its returned bytes exercises
	// ownership as well as reuse of the fixed plan, including after
	// failure.
	for range 2 {
		result, err := plan.Satisfy(c.Data.satisfier(t))
		if c.Expected.Error == "satisfy" {
			require.Error(t, err)

			continue
		}

		// Compare every assembled byte before checking signatures.
		// Correct Script execution alone would not detect an unexpected
		// branch or multisig subset that also happens to be spendable.
		require.Empty(t, c.Expected.Error)
		require.NoError(t, err)
		require.Equal(t, c.Expected.ScriptSig, hex.EncodeToString(
			result.ScriptSig,
		))
		require.Len(t, result.Witness, len(c.Expected.Witness))
		for i, item := range result.Witness {
			require.Equal(
				t, c.Expected.Witness[i],
				hex.EncodeToString(item), "witness item %d", i,
			)
		}

		// Assembly-only fixtures may use synthetic signatures. Execute
		// only the explicitly signed cases and require their validity
		// expectation so a missing field cannot silently weaken a
		// verification test.
		if c.Verify {
			require.NotNil(t, v.Transaction)
			require.NotNil(t, c.Expected.Valid)
			require.Equal(
				t, *c.Expected.Valid,
				v.Transaction.verify(t, result),
				"script verification",
			)
		} else {
			require.Nil(t, c.Expected.Valid)
		}

		// A caller owns returned bytes. Damaging them must not damage
		// either this plan or the descriptor's future completions.
		for _, item := range result.Witness {
			for i := range item {
				item[i] ^= 0xff
			}
		}
		for i := range result.ScriptSig {
			result.ScriptSig[i] ^= 0xff
		}
	}
}

// vectorBytes decodes fixture hex without silently accepting malformed data.
func vectorBytes(t *testing.T, value string) []byte {
	t.Helper()

	// Malformed hex is a fixture failure, not unavailable signing data that
	// could accidentally make a negative satisfaction vector pass.
	b, err := hex.DecodeString(value)
	require.NoError(t, err)

	return b
}

// assets translates lookup tables without changing availability or sizes.
// Missing transaction fields remain unknown, and no completion data is used.
func (v spendingVector) assets() descriptors.Assets {
	// Use exact lookup identities. Availability for one key, Taproot leaf
	// or hash function must not accidentally make another spending path
	// usable.
	return descriptors.Assets{
		TxVersion:       v.Tx.Version,
		TxLockTime:      v.Tx.LockTime,
		TxInputSequence: v.Tx.Sequence,
		LookupEcdsaSig: func(key string) bool {
			return slices.Contains(v.Assets.ECDSA, key)
		},
		LookupTapKeySpendSig: func(key string) (uint32, bool) {
			for _, a := range v.Assets.TapKey {
				if a.Key == key {
					return a.Size, true
				}
			}

			return 0, false
		},
		LookupTapLeafScriptSig: func(key, leaf string) (uint32, bool) {
			for _, a := range v.Assets.TapLeaf {
				if a.Key == key && a.LeafHash == leaf {
					return a.Size, true
				}
			}

			return 0, false
		},
		LookupPreimage: func(function string, digest []byte) bool {
			for _, a := range v.Assets.Preimages {
				if a.Function == function &&
					a.Hash == hex.EncodeToString(digest) {

					return true
				}
			}

			return false
		},
	}
}

// satisfier translates concrete vector data without consulting the plan assets.
// Each successful lookup decodes fresh bytes, and malformed hex fails the test.
func (v vectorData) satisfier(t *testing.T) *descriptors.Satisfier {
	t.Helper()

	// Keep availability separate from the plan's original assets.
	// Completion fixtures deliberately omit selected data or supply extra
	// signatures to check that the plan cannot silently choose another
	// satisfaction.
	return &descriptors.Satisfier{
		LookupEcdsaSig: func(key string) ([]byte, bool) {
			sig, ok := v.ECDSA[key]

			return vectorBytes(t, sig), ok
		},
		LookupTapKeySpendSig: func() ([]byte, bool) {
			// The Go key-spend callback has no key argument: this
			// map must describe the descriptor's single internal
			// key.
			require.LessOrEqual(t, len(v.TapKey), 1)
			for _, sig := range v.TapKey {
				return vectorBytes(t, sig), true
			}

			return nil, false
		},
		LookupTapLeafScriptSig: func(key, leaf string) ([]byte, bool) {
			for _, sig := range v.TapLeaf {
				if sig.Key == key && sig.LeafHash == leaf {
					return vectorBytes(
						t, sig.Signature,
					), true
				}
			}

			return nil, false
		},
		LookupPreimage: func(function string, digest []byte) ([]byte,
			bool) {

			for _, a := range v.Preimages {
				if a.Function == function &&
					a.Hash == hex.EncodeToString(digest) {

					return vectorBytes(t, a.Preimage), true
				}
			}

			return nil, false
		},
	}
}

// TestSpendingVectors checks the shared plan and satisfaction fixtures using
// only exported descriptor APIs. Expected bytes are never derived by the code
// under test. Every completion reuses the same plan, including after failures.
func TestSpendingVectors(t *testing.T) {
	// Reject schema drift and trailing JSON before running cases. Silently
	// ignoring a new expectation could make an incomplete harness pass.
	contents, err := os.ReadFile("testdata/spending_vectors.json")
	require.NoError(t, err)
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var vectors spendingVectors
	require.NoError(t, decoder.Decode(&vectors))
	require.ErrorIs(t, decoder.Decode(new(any)), io.EOF)
	require.Equal(t, 1, vectors.Version)
	require.NotEmpty(t, vectors.Cases)

	// Stable, unique IDs keep failures attributable to the portable vectors
	// instead of letting the test runner rename duplicate cases implicitly.
	seen := make(map[string]bool)
	for _, v := range vectors.Cases {
		require.False(t, seen[v.ID], "duplicate vector: %s", v.ID)
		seen[v.ID] = true
		t.Run(v.ID, func(t *testing.T) {
			// Check parsing separately: rejection at the wrong
			// stage must not satisfy a planning or completion error
			// vector.
			d, err := descriptors.NewDescriptor(v.Descriptor)
			if v.ExpectedPlan.Error == "parse" {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)

			// Select the plan using only advertised assets and
			// locktime context. Concrete completion data must not
			// influence it.
			plan, err := d.PlanAt(
				v.MultipathIndex, v.DerivationIndex, v.assets(),
			)
			if v.ExpectedPlan.Error == "plan" {
				require.Error(t, err)

				return
			}
			require.Empty(t, v.ExpectedPlan.Error)
			require.NoError(t, err)

			// Assert each serialized size as well as total weight
			// so an incorrect witness/scriptSig split cannot go
			// unnoticed.
			require.Equal(
				t, v.ExpectedPlan.WitnessSize,
				plan.WitnessSize(), "witness size",
			)
			require.Equal(
				t, v.ExpectedPlan.ScriptSigSize,
				plan.ScriptSigSize(), "scriptSig size",
			)
			require.Equal(
				t, v.ExpectedPlan.Weight,
				plan.SatisfactionWeight(), "weight",
			)

			// Verify the derived output through the public API.
			// Bare scripts have no address; other descriptor forms
			// expose their output through address conversion.
			var output []byte
			if d.DescType() == descriptors.DescTypeBare {
				output, err = d.ScriptCodeAt(
					v.MultipathIndex, v.DerivationIndex,
				)
			} else {
				var encoded string
				encoded, err = d.AddressAt(
					&chaincfg.MainNetParams,
					v.MultipathIndex, v.DerivationIndex,
				)
				require.NoError(t, err)
				addr, decodeErr := address.DecodeAddress(
					encoded, &chaincfg.MainNetParams,
				)
				require.NoError(t, decodeErr)
				output, err = txscript.PayToAddrScript(addr)
			}
			require.NoError(t, err)
			require.Equal(
				t, v.ScriptPubKey, hex.EncodeToString(output),
				"output script",
			)

			// Reuse this exact plan across all attempts, including
			// failures, to catch fallback to a different spending
			// path or corruption of the plan by an earlier
			// completion.
			for _, c := range v.Completions {
				t.Run(c.ID, func(t *testing.T) {
					checkVectorCompletion(t, plan, v, c)
				})
			}
		})
	}
}

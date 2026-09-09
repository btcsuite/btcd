package miniscript

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

var (
	// semanticSeeds exercises individual guards, both binary operators and
	// nested mixed choices. Fuzzing changes structure while retaining a
	// bounded grammar.
	semanticSeeds = [][]byte{
		{0, 0}, {0, 1}, {0, 2}, {0, 3},
		{1, 0, 0, 0, 1}, {2, 0, 2, 0, 3},
		{1, 2, 0, 0, 0, 1, 2, 0, 2, 0, 3},
		{2, 1, 0, 0, 0, 1, 1, 0, 2, 0, 3},
	}
)

// semanticNode is a deliberately small test language, independent of AST and
// Policy. Every leaf requires its own signature, optionally guarded by a hash
// or height lock. AND and explicit-choice OR preserve non-malleable spending
// completeness; this model makes no claim about arbitrary insane miniscripts.
type semanticNode struct {
	op          byte
	key         byte
	guard       byte
	left, right *semanticNode
}

// semanticTree decodes bounded structural choices, not miniscript text. Depth
// two gives at most four distinct keys; exhaustion deterministically adds keys.
func semanticTree(data []byte) *semanticNode {
	position, key := 0, byte(0)
	next := func() byte {
		if position >= len(data) {
			return 0
		}
		value := data[position]
		position++
		return value
	}
	var build func(int) *semanticNode
	build = func(depth int) *semanticNode {
		op := next() % 3
		if depth == 2 || op == 0 {
			node := &semanticNode{key: key, guard: next() % 4}
			key++
			return node
		}
		return &semanticNode{
			op: op,
			left: build(
				depth + 1,
			),
			right: build(
				depth + 1,
			),
		}
	}
	return build(0)
}

// expression renders only known well-typed constructions. Symbolic key names
// let the same model run with compressed segwit keys and x-only taproot keys.
func (n *semanticNode) expression() string {
	switch n.op {
	case 1:
		return fmt.Sprintf("and_v(v:%s,%s)", n.left.expression(), n.right.expression())

	case 2:
		return fmt.Sprintf("or_i(%s,%s)", n.left.expression(), n.right.expression())

	default:
		key := fmt.Sprintf("pk(%c)", 'A'+n.key)
		switch n.guard {
		case 1:
			return fmt.Sprintf("and_v(v:%s,sha256(%x))", key, chainhash.HashB(semanticPreimage()))

		case 2:
			return "and_v(v:" + key + ",older(2))"

		case 3:
			return "and_v(v:" + key + ",after(2))"

		default:
			return key
		}
	}
}

// semanticAssets records the independently chosen signing and transaction
// facts. Locks in this grammar are height-based and always have operand two.
type semanticAssets struct {
	keys               uint8
	preimage           bool
	version            int32
	sequence, locktime uint32
}

// guardAvailable evaluates the restricted language directly. In particular it
// does not call CheckOlder or CheckAfter, which are part of the code under
// test.
func (a semanticAssets) guardAvailable(guard byte) bool {
	switch guard {
	case 1:
		return a.preimage

	case 2:
		// BIP112: version two, neither disabled nor time-based
		// sequence, and a sufficient low-16-bit relative height.
		return a.version >= 2 && a.sequence&(1<<31|1<<22) == 0 &&
			a.sequence&0xffff >= 2

	case 3:
		// BIP65: matching height units, sufficient absolute height and
		// a non-final input sequence so nLockTime is actually enabled.
		return a.locktime >= 2 && a.locktime < 500000000 &&
			a.sequence != 0xffffffff

	default:
		return true
	}
}

// possible is the reference truth table, evaluated without any production
// parsing, type analysis, lifting, path selection or satisfaction helpers.
func (n *semanticNode) possible(a semanticAssets) bool {
	switch n.op {
	case 1:
		return n.left.possible(a) && n.right.possible(a)

	case 2:
		return n.left.possible(a) || n.right.possible(a)

	default:
		return a.keys&(1<<n.key) != 0 && a.guardAvailable(n.guard)
	}
}

// semanticPolicyTruth interprets the output of Lift, allowing its meaning to
// be compared with the input model rather than with another normalized policy.
func semanticPolicyTruth(t *testing.T, p *Policy, a semanticAssets) bool {
	t.Helper()
	switch p.Type {
	case PolicyKey:
		require.Len(t, p.Key, 1)
		require.Contains(t, "ABCD", p.Key)
		return a.keys&(1<<(p.Key[0]-'A')) != 0

	case PolicyOlder, PolicyAfter:
		require.EqualValues(t, 2, p.Locktime)
		if p.Type == PolicyOlder {
			return a.guardAvailable(2)
		}
		return a.guardAvailable(3)

	case PolicySha256:
		require.Equal(t, chainhash.HashB(semanticPreimage()), p.Hash)
		return a.preimage

	case PolicyThresh:
		// General thresholds are interpreted literally, without
		// flattening, sorting or calling production normalization.
		count := 0
		for _, child := range p.Subs {
			if semanticPolicyTruth(t, child, a) {
				count++
			}
		}
		return count >= p.K

	default:
		require.FailNow(t, "unexpected lifted node", "%v", p.Type)
		return false
	}
}

// semanticPreimage returns fresh storage so a satisfaction cannot affect the
// reference commitment or the input to a later test execution.
func semanticPreimage() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}

// checkSemanticSpend checks both success and refusal against the independent
// model, then executes every successful witness and checks its resource bounds.
func checkSemanticSpend(t *testing.T, model *semanticNode, a semanticAssets,
	ctx Context) {

	t.Helper()
	expression := model.expression()
	node, err := Parse(expression, ctx)
	require.NoError(t, err, expression)
	policy, err := node.Lift()
	require.NoError(t, err)
	want := model.possible(a)
	require.Equal(t, want, semanticPolicyTruth(t, policy, a), expression)
	require.Equal(t, policy, policy.Normalize())

	// Deterministic private scalars produce distinct real keys. The model
	// speaks only in key indices, never in production AST properties.
	private := make(map[string]*btcec.PrivateKey)
	available := make(map[string]bool)
	public := make(map[string][]byte)
	for i := range byte(4) {
		key, pub := btcec.PrivKeyFromBytes([]byte{i + 1})
		serialized := pub.SerializeCompressed()
		if ctx == P2TR {
			serialized = schnorr.SerializePubKey(pub)
		}
		name := string(rune('A' + i))
		public[name] = serialized
		private[string(serialized)] = key
		available[string(serialized)] = a.keys&(1<<i) != 0
	}
	require.NoError(t, node.ApplyVars(func(name string) ([]byte, error) {
		return public[name], nil
	}))
	script, err := node.Script()
	require.NoError(t, err)
	require.Equal(t, len(script), node.ScriptLen())

	// Commit the generated script to a real witness output. Taproot uses a
	// separate internal key; only the modeled script path is available.
	pkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_0).
		AddData(chainhash.HashB(script)).Script()
	require.NoError(t, err)
	leaf := txscript.NewBaseTapLeaf(script)
	var control []byte
	if ctx == P2TR {
		_, internal := btcec.PrivKeyFromBytes([]byte{200})
		tree := txscript.AssembleTaprootScriptTree(leaf)
		root := tree.RootNode.TapHash()
		output := txscript.ComputeTaprootOutputKey(internal, root[:])
		pkScript, err = txscript.PayToTaprootScript(output)
		require.NoError(t, err)
		block := tree.LeafMerkleProofs[0].ToControlBlock(internal)
		control, err = block.ToBytes()
		require.NoError(t, err)
	}
	const amount = int64(10000)
	tx := wire.NewMsgTx(a.version)
	tx.LockTime = a.locktime
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, nil))
	tx.TxIn[0].Sequence = a.sequence
	tx.AddTxOut(wire.NewTxOut(amount-1000, []byte{txscript.OP_RETURN}))
	prevouts := txscript.NewCannedPrevOutputFetcher(pkScript, amount)
	hashes := txscript.NewTxSigHashes(tx, prevouts)

	// Call the real transaction-lock helpers and sign precisely this
	// transaction. Missing assets must cause refusal, not a skipped test.
	witness, err := node.Satisfy(&Satisfier{
		Sign: func(pub []byte) ([]byte, bool) {
			if !available[string(pub)] {
				return nil, false
			}
			var signature []byte
			var err error
			if ctx == P2TR {
				signature, err = txscript.RawTxInTapscriptSignature(tx, hashes, 0, amount, pkScript, leaf, txscript.SigHashDefault, private[string(pub)])
			} else {
				signature, err = txscript.RawTxInWitnessSignature(tx, hashes, 0, amount, script, txscript.SigHashAll, private[string(pub)])
			}
			require.NoError(t, err)
			return signature, true
		},
		Preimage: func(function string, hash []byte) ([]byte, bool) {
			require.Equal(t, "sha256", function)
			require.Equal(
				t, chainhash.HashB(semanticPreimage()), hash,
			)
			return semanticPreimage(), a.preimage
		},
		CheckOlder: func(lock uint32) (bool, error) {
			return CheckOlder(lock, a.version, a.sequence), nil
		},
		CheckAfter: func(lock uint32) (bool, error) {
			return CheckAfter(lock, a.locktime, a.sequence), nil
		},
	})
	if !want {
		require.Error(t, err, "%s %+v", expression, a)
		return
	}
	require.NoError(t, err, "%s %+v", expression, a)
	bound, err := node.MaxSatisfactionSize()
	require.NoError(t, err)
	require.LessOrEqual(t, witnessSize(witness), bound)
	require.LessOrEqual(t, len(witness), node.maxWitnessSize())

	// Step through execution to observe both main and alternate stacks.
	// The witness program itself can temporarily require two elements.
	stackBound := max(2, len(witness)+node.maxExecStackSize())
	tx.TxIn[0].Witness = append(witness, script)
	if ctx == P2TR {
		tx.TxIn[0].Witness = append(tx.TxIn[0].Witness, control)
	}
	engine, err := txscript.NewEngine(
		pkScript, tx, 0, txscript.StandardVerifyFlags, nil, hashes,
		amount, prevouts,
	)
	require.NoError(t, err)
	for {
		done, err := engine.Step()
		require.NoError(t, err, expression)
		require.LessOrEqual(
			t, len(engine.GetStack())+len(engine.GetAltStack()),
			stackBound,
		)
		if done {
			break
		}
	}
	require.NoError(t, engine.CheckErrorCondition(true))
}

// TestSemanticSpending enumerates every signer/preimage subset for each seed
// with boundary transaction contexts, including version and lock-unit mismatch.
func TestSemanticSpending(t *testing.T) {
	t.Parallel()
	for _, seed := range semanticSeeds {
		model := semanticTree(seed)
		for _, ctx := range []Context{P2WSH, P2TR} {
			for _, facts := range []semanticAssets{
				{version: 2, sequence: 1, locktime: 1},
				{version: 2, sequence: 2, locktime: 2},
				{version: 1, sequence: 2, locktime: 2},
				{
					version:  2,
					sequence: 1<<22 | 2,
					locktime: 500000000,
				},
				{version: 2, sequence: 0xffffffff, locktime: 2},
			} {

				for mask := range uint8(32) {
					facts.keys, facts.preimage = mask&15, mask&16 != 0
					checkSemanticSpend(t, model, facts, ctx)
				}
			}
		}
	}
}

// FuzzSemanticSpending mutates structural choices and availability, checking
// real spends rather than spending most fuzz executions on invalid syntax.
func FuzzSemanticSpending(f *testing.F) {
	for _, seed := range semanticSeeds {
		f.Add(seed, uint8(31), uint32(2), uint32(2), true)
	}
	f.Fuzz(
		func(t *testing.T, data []byte, mask uint8,
			sequence, locktime uint32, tap bool) {

			// Only a bounded prefix contributes to the tree. The
			// transaction fields remain unconstrained to reach flag
			// and unit boundaries.
			model := semanticTree(data)
			ctx := P2WSH
			if tap {
				ctx = P2TR
			}
			facts := semanticAssets{
				keys:     mask & 15,
				preimage: mask&16 != 0,
				version: 1 + int32(
					mask>>5&1,
				),
				sequence: sequence,
				locktime: locktime,
			}
			checkSemanticSpend(t, model, facts, ctx)
		},
	)
}

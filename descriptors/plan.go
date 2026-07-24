package descriptors

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/btcsuite/btcd/txscript/v2"
)

const (
	// assumedEcdsaSigLen is the assumed size in bytes of an ECDSA
	// signature (a DER signature plus its sighash byte), used when
	// estimating a plan's weight. It matches rust-miniscript's assumption.
	assumedEcdsaSigLen = 72

	// tapLeafVersion is the base tapscript leaf version, used as the first
	// byte of a control block (combined with the output key parity).
	tapLeafVersion = byte(txscript.BaseLeafVersion)
)

var (
	// errCouldNotPlan is returned by PlanAt when the given assets are
	// insufficient to produce a satisfaction.
	errCouldNotPlan = errors.New("could not create plan")

	// errCouldNotSatisfy is returned by Plan.Satisfy when the satisfier
	// cannot provide the data the plan requires.
	errCouldNotSatisfy = errors.New("could not satisfy")
)

// Assets describes the present/missing lookup table used to construct a plan's
// witness template. Any nil lookup is treated as "not available".
type Assets struct {
	// LookupEcdsaSig reports whether an ECDSA signature is available for
	// the given public key.
	LookupEcdsaSig func(pk string) bool

	// LookupTapKeySpendSig reports whether a taproot key-spend signature is
	// available for the given public key, and its size.
	LookupTapKeySpendSig func(pk string) (uint32, bool)

	// LookupTapLeafScriptSig reports whether a taproot leaf-script
	// signature is available for the given public key and leaf hash, and
	// its size.
	LookupTapLeafScriptSig func(pk string, leafHash string) (uint32, bool)

	// RelativeLocktime is the maximum relative locktime allowed.
	RelativeLocktime *uint32

	// AbsoluteLocktime is the maximum absolute locktime allowed.
	AbsoluteLocktime *uint32
}

// Satisfier provides the concrete signatures used to complete a plan.
type Satisfier struct {
	// LookupEcdsaSig returns a DER-encoded ECDSA signature (including the
	// sighash type byte) for the given public key.
	LookupEcdsaSig func(pk string) ([]byte, bool)

	// LookupTapKeySpendSig returns a taproot key-spend signature.
	LookupTapKeySpendSig func() ([]byte, bool)

	// LookupTapLeafScriptSig returns a taproot leaf-script signature for
	// the given public key and leaf hash.
	LookupTapLeafScriptSig func(pk string, leafHash string) ([]byte, bool)
}

// SatisfyResult is the completed witness and scriptSig produced by a plan.
type SatisfyResult struct {
	// Witness is the witness stack.
	Witness [][]byte

	// ScriptSig is the scriptSig bytes.
	ScriptSig []byte
}

// Plan is a chosen spending path on a descriptor: it captures the weight of the
// satisfaction and knows how to complete it from a Satisfier.
type Plan struct {
	// witnessSize is the size in bytes of the witness (0 for non-segwit
	// outputs).
	witnessSize uint64

	// scriptSigSize is the size in bytes of the scriptSig, including its
	// var-int length prefix.
	scriptSigSize uint64

	// satisfy completes the plan from a satisfier.
	satisfy func(*Satisfier) (*SatisfyResult, error)
}

// SatisfactionWeight returns the weight, in weight units, needed to satisfy
// this plan (both the scriptSig and the witness).
func (p *Plan) SatisfactionWeight() uint64 {
	return p.witnessSize + p.scriptSigSize*4
}

// ScriptSigSize returns the size in bytes of the scriptSig that satisfies this
// plan, including its var-int length prefix.
func (p *Plan) ScriptSigSize() uint64 {
	return p.scriptSigSize
}

// WitnessSize returns the size in bytes of the witness that satisfies this
// plan.
func (p *Plan) WitnessSize() uint64 {
	return p.witnessSize
}

// Satisfy completes the plan, producing the final witness and scriptSig, or an
// error if the satisfier cannot provide the required data.
func (p *Plan) Satisfy(satisfier *Satisfier) (*SatisfyResult, error) {
	return p.satisfy(satisfier)
}

// PlanAt returns a plan for the given multipath and derivation index if the
// provided assets are sufficient to produce a non-malleable satisfaction.
func (d *Descriptor) PlanAt(multipathIndex, derivationIndex uint32,
	assets Assets) (*Plan, error) {

	if uint64(multipathIndex) >= uint64(d.multipath) {
		return nil, fmt.Errorf("multipath index out of bounds")
	}
	return d.planNode(d.root, multipathIndex, derivationIndex, assets)
}

// planNode builds a plan for a descriptor's top-level node.
func (d *Descriptor) planNode(n *node, mp, idx uint32, assets Assets) (*Plan,
	error) {

	switch n.kind {
	case nodeTr:
		return d.planTr(n, mp, idx, assets)

	case nodeWsh:
		return d.planWitnessScript(n.sub, mp, idx, assets, 1, nil)

	case nodeSh:
		return d.planSh(n.sub, mp, idx, assets)

	case nodeWpkh:
		return d.planKeyHash(n, mp, idx, assets, true, 1, nil)

	case nodePkh:
		return d.planKeyHash(n, mp, idx, assets, false, 0, nil)

	default:
		// A bare script: the whole satisfaction is the scriptSig.
		return d.planBare(n, mp, idx, assets)
	}
}

// planSh builds a plan for a P2SH output, dispatching on its wrapped inner.
func (d *Descriptor) planSh(sub *node, mp, idx uint32, assets Assets) (*Plan,
	error) {

	switch sub.kind {
	case nodeWsh:
		// The scriptSig is a push of the P2WSH redeem script.
		witnessScript, err := d.innerScript(sub.sub, mp, idx)
		if err != nil {
			return nil, err
		}
		redeem, err := witnessV0Script(chainhash.HashB(witnessScript))
		if err != nil {
			return nil, err
		}
		scriptSig, err := pushScript(redeem)
		if err != nil {
			return nil, err
		}
		return d.planWitnessScript(
			sub.sub, mp, idx, assets, uint64(len(scriptSig)),
			scriptSig,
		)

	case nodeWpkh:
		program, err := d.wpkhProgram(sub, mp, idx)
		if err != nil {
			return nil, err
		}
		redeem, err := witnessV0Script(program)
		if err != nil {
			return nil, err
		}
		scriptSig, err := pushScript(redeem)
		if err != nil {
			return nil, err
		}
		return d.planKeyHash(
			sub, mp, idx, assets, true, uint64(len(scriptSig)),
			scriptSig,
		)

	default:
		// A legacy P2SH: the redeem script and satisfaction are both in
		// the scriptSig, matching a bare script's weight accounting.
		return d.planBare(sub, mp, idx, assets)
	}
}

// planWitnessScript builds a plan whose satisfaction is the miniscript
// satisfaction of a witness script (a wsh inner or a P2SH-wrapped wsh inner).
// scriptSig is the (possibly empty) scriptSig; scriptSigSize its accounted
// size.
func (d *Descriptor) planWitnessScript(sub *node, mp, idx uint32, assets Assets,
	scriptSigSize uint64, scriptSig []byte) (*Plan, error) {

	template, err := d.satisfyScript(
		sub, mp, idx, assets, planEcdsa(assets),
	)
	if err != nil {
		return nil, errCouldNotPlan
	}

	if scriptSig == nil {
		scriptSig = []byte{}
	}

	return &Plan{
		witnessSize:   witnessSerializedSize(template),
		scriptSigSize: scriptSigSize,
		satisfy: func(s *Satisfier) (*SatisfyResult, error) {
			witness, err := d.satisfyScript(
				sub, mp, idx, assets, realEcdsa(s),
			)
			if err != nil {
				return nil, errCouldNotSatisfy
			}
			return &SatisfyResult{
				Witness:   witness,
				ScriptSig: scriptSig,
			}, nil
		},
	}, nil
}

// planBare builds a plan for a bare or legacy-P2SH script, where the entire
// satisfaction lives in the scriptSig.
func (d *Descriptor) planBare(sub *node, mp, idx uint32, assets Assets) (*Plan,
	error) {

	template, err := d.satisfyScript(
		sub, mp, idx, assets, planEcdsa(assets),
	)
	if err != nil {
		return nil, errCouldNotPlan
	}

	return &Plan{
		witnessSize:   0,
		scriptSigSize: witnessSerializedSize(template),
		satisfy: func(s *Satisfier) (*SatisfyResult, error) {
			witness, err := d.satisfyScript(
				sub, mp, idx, assets, realEcdsa(s),
			)
			if err != nil {
				return nil, errCouldNotSatisfy
			}
			scriptSig, err := pushAll(witness)
			if err != nil {
				return nil, err
			}
			return &SatisfyResult{
				Witness:   [][]byte{},
				ScriptSig: scriptSig,
			}, nil
		},
	}, nil
}

// planKeyHash builds a plan for a single-key output (wpkh, pkh or their
// P2SH-wrapped variants). segwit selects whether the signature and key go in
// the witness or the scriptSig.
func (d *Descriptor) planKeyHash(n *node, mp, idx uint32, assets Assets,
	segwit bool, scriptSigSize uint64, scriptSig []byte) (*Plan, error) {

	key := n.keys[0]
	defKey := key.definiteString(mp, idx)
	if assets.LookupEcdsaSig == nil || !assets.LookupEcdsaSig(defKey) {
		return nil, errCouldNotPlan
	}

	pubKey, err := key.derive(mp, idx, false)
	if err != nil {
		return nil, err
	}

	// The witness template is <sig> <pubkey>.
	template := [][]byte{make([]byte, assumedEcdsaSigLen), pubKey}
	size := witnessSerializedSize(template)

	if scriptSig == nil {
		scriptSig = []byte{}
	}

	plan := &Plan{}
	if segwit {
		plan.witnessSize = size
		plan.scriptSigSize = scriptSigSize
	} else {
		plan.witnessSize = 0
		plan.scriptSigSize = size
	}

	plan.satisfy = func(s *Satisfier) (*SatisfyResult, error) {
		if s.LookupEcdsaSig == nil {
			return nil, errCouldNotSatisfy
		}
		sig, ok := s.LookupEcdsaSig(defKey)
		if !ok {
			return nil, errCouldNotSatisfy
		}
		if segwit {
			return &SatisfyResult{
				Witness:   [][]byte{sig, pubKey},
				ScriptSig: scriptSig,
			}, nil
		}
		scriptSig, err := pushAll([][]byte{sig, pubKey})
		if err != nil {
			return nil, err
		}
		return &SatisfyResult{
			Witness:   [][]byte{},
			ScriptSig: scriptSig,
		}, nil
	}

	return plan, nil
}

// planTr builds a plan for a taproot output, choosing the cheapest of the key
// path and each script-tree leaf that the assets can satisfy.
func (d *Descriptor) planTr(n *node, mp, idx uint32, assets Assets) (*Plan,
	error) {

	var candidates []*Plan

	// Key-path candidate.
	internalDef := n.keys[0].definiteString(mp, idx)
	if assets.LookupTapKeySpendSig != nil {
		if size, ok := assets.LookupTapKeySpendSig(internalDef); ok {
			template := [][]byte{make([]byte, size)}
			candidates = append(candidates, &Plan{
				witnessSize:   witnessSerializedSize(template),
				scriptSigSize: 1,
				satisfy:       trKeySpendSatisfy(),
			})
		}
	}

	// Script-path candidates: one per leaf of the tree.
	if n.tapTree != nil {
		leaves, _, err := d.collectLeafPlans(n.tapTree, mp, idx)
		if err != nil {
			return nil, err
		}

		// Every control block starts with the leaf version combined
		// with the output key parity, then the x-only internal key.
		outputKey, err := d.taprootOutputKey(n, mp, idx)
		if err != nil {
			return nil, err
		}
		internal, err := n.keys[0].derive(mp, idx, true)
		if err != nil {
			return nil, err
		}
		version := tapLeafVersion
		if outputKey.SerializeCompressed()[0] == 0x03 {
			version |= 1
		}

		for _, lp := range leaves {
			controlBlock := make([]byte, 0, 33+len(lp.proof))
			controlBlock = append(controlBlock, version)
			controlBlock = append(controlBlock, internal...)
			controlBlock = append(controlBlock, lp.proof...)

			plan, err := d.planTrLeaf(
				lp, controlBlock, mp, idx, assets,
			)
			if err == nil {
				candidates = append(candidates, plan)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, errCouldNotPlan
	}

	// Choose the candidate with the lowest satisfaction weight.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.SatisfactionWeight() < best.SatisfactionWeight() {
			best = c
		}
	}
	return best, nil
}

// trKeySpendSatisfy returns the satisfy closure for a taproot key-path spend.
func trKeySpendSatisfy() func(*Satisfier) (*SatisfyResult, error) {
	return func(s *Satisfier) (*SatisfyResult, error) {
		if s.LookupTapKeySpendSig == nil {
			return nil, errCouldNotSatisfy
		}
		sig, ok := s.LookupTapKeySpendSig()
		if !ok {
			return nil, errCouldNotSatisfy
		}
		return &SatisfyResult{
			Witness:   [][]byte{sig},
			ScriptSig: []byte{},
		}, nil
	}
}

// leafPlan holds the derived data needed to spend one tapscript leaf: its
// miniscript node, its compiled script and the merkle path (ordered from the
// leaf's sibling up toward the root) proving its position in the tree.
type leafPlan struct {
	leaf   *node
	script []byte
	proof  []byte
}

// planTrLeaf builds the script-path candidate plan for one tapscript leaf,
// given its fully-assembled control block.
func (d *Descriptor) planTrLeaf(lp leafPlan, controlBlock []byte, mp,
	idx uint32, assets Assets) (*Plan, error) {

	// Tap leaf hashes are displayed in forward byte order, unlike the
	// reversed order used for transaction and block hashes.
	leafHashBytes := txscript.NewBaseTapLeaf(lp.script).TapHash()
	leafHash := hex.EncodeToString(leafHashBytes[:])

	leafSat, err := d.satisfyScript(
		lp.leaf, mp, idx, assets, planTapLeaf(assets, leafHash),
	)
	if err != nil {
		return nil, err
	}

	template := append(leafSat, lp.script, controlBlock)

	return &Plan{
		witnessSize:   witnessSerializedSize(template),
		scriptSigSize: 1,
		satisfy: func(s *Satisfier) (*SatisfyResult, error) {
			witness, err := d.satisfyScript(
				lp.leaf, mp, idx, assets,
				realTapLeaf(s, leafHash),
			)
			if err != nil {
				return nil, errCouldNotSatisfy
			}
			witness = append(witness, lp.script, controlBlock)
			return &SatisfyResult{
				Witness:   witness,
				ScriptSig: []byte{},
			}, nil
		},
	}, nil
}

// collectLeafPlans walks a taproot script tree and returns, for every leaf, its
// compiled script and merkle path, along with the tree's merkle root. Each
// path is ordered from the leaf's sibling up toward the root.
func (d *Descriptor) collectLeafPlans(t *tapTree, mp, idx uint32) ([]leafPlan,
	chainhash.Hash, error) {

	if t.leaf != nil {
		script, err := d.innerScript(t.leaf, mp, idx)
		if err != nil {
			return nil, chainhash.Hash{}, err
		}
		hash := txscript.NewBaseTapLeaf(script).TapHash()
		return []leafPlan{{leaf: t.leaf, script: script}}, hash, nil
	}

	left, leftHash, err := d.collectLeafPlans(t.left, mp, idx)
	if err != nil {
		return nil, chainhash.Hash{}, err
	}
	right, rightHash, err := d.collectLeafPlans(t.right, mp, idx)
	if err != nil {
		return nil, chainhash.Hash{}, err
	}

	// Each leaf of one side gains the other side's subtree hash as the next
	// (shallower) element of its merkle path.
	for i := range left {
		left[i].proof = append(left[i].proof, rightHash[:]...)
	}
	for i := range right {
		right[i].proof = append(right[i].proof, leftHash[:]...)
	}

	return append(left, right...), branchHash(leftHash, rightHash), nil
}

// branchHash computes the tagged hash of a taproot branch, sorting the two
// child hashes lexicographically as required by BIP341.
func branchHash(a, b chainhash.Hash) chainhash.Hash {
	left, right := a[:], b[:]
	if bytes.Compare(left, right) > 0 {
		left, right = right, left
	}
	return *chainhash.TaggedHash(chainhash.TagTapBranch, left, right)
}

// satisfyScript runs the miniscript satisfaction of a wsh inner or tapscript
// leaf node, resolving signatures through the given lookup keyed by the
// definite key string. It returns the witness stack (excluding the witness
// script).
func (d *Descriptor) satisfyScript(n *node, mp, idx uint32, assets Assets,
	lookup func(defKey string) ([]byte, bool)) ([][]byte, error) {

	ast, ctx, err := d.planAST(n, mp, idx)
	if err != nil {
		return nil, err
	}

	xOnly := ctx == miniscript.P2TR
	err = ast.ApplyVars(func(id string) ([]byte, error) {
		k, ok := d.keyByRaw[id]
		if !ok {
			return nil, fmt.Errorf("unknown key %q", id)
		}
		return k.derive(mp, idx, xOnly)
	})
	if err != nil {
		return nil, err
	}

	// Map each derived public key back to its descriptor key so the sign
	// callback can resolve the definite key string it was asked to look up.
	pkMap := make(map[string]*descKey)
	for _, k := range d.keys {
		pub, err := k.derive(mp, idx, xOnly)
		if err != nil {
			continue
		}
		pkMap[string(pub)] = k
	}

	satisfier := &miniscript.Satisfier{
		Sign: func(pubKey []byte) ([]byte, bool) {
			k, ok := pkMap[string(pubKey)]
			if !ok {
				return nil, false
			}
			return lookup(k.definiteString(mp, idx))
		},
		CheckOlder: func(lt uint32) (bool, error) {
			return relativeImplied(lt, assets.RelativeLocktime), nil
		},
		CheckAfter: func(lt uint32) (bool, error) {
			return absoluteImplied(lt, assets.AbsoluteLocktime), nil
		},
		Preimage: func(string, []byte) ([]byte, bool) {
			return nil, false
		},
	}

	return ast.Satisfy(satisfier)
}

// planAST returns the parsed miniscript AST and context for a wsh/sh inner or
// tapscript leaf node, so it can be satisfied through the miniscript engine.
// For a miniscript node the AST is cloned from the one cached at construction
// time; the pk/pkh/multi/sortedmulti nodes are emitted as an equivalent
// miniscript expression and parsed on demand, since a sortedmulti's key order
// depends on the multipath and derivation index. The returned AST is always a
// fresh, mutable copy safe to pass to ApplyVars/Satisfy.
func (d *Descriptor) planAST(n *node, mp, idx uint32) (*miniscript.AST,
	miniscript.Context, error) {

	if n.kind == nodeMs {
		return n.clonedMsAST(), n.msCtx, nil
	}

	expr, ctx, err := d.planExpr(n, mp, idx)
	if err != nil {
		return nil, ctx, err
	}

	ast, err := miniscript.Parse(expr, ctx)
	if err != nil {
		return nil, ctx, err
	}

	return ast, ctx, nil
}

// planExpr returns the miniscript expression and context for a pk/pkh/multi/
// sortedmulti wsh/sh inner or tapscript leaf node, so it can be satisfied
// through the miniscript engine. A sortedmulti is emitted as a multi with its
// keys in the BIP67 order they take at the given multipath and derivation
// index.
func (d *Descriptor) planExpr(n *node, mp, idx uint32) (string,
	miniscript.Context, error) {

	switch n.kind {
	case nodePk:
		return "pk(" + n.keys[0].raw + ")", miniscript.P2WSH, nil

	case nodePkh:
		return "pkh(" + n.keys[0].raw + ")", miniscript.P2WSH, nil

	case nodeMulti, nodeSortedMulti:
		keys := n.keys
		if n.kind == nodeSortedMulti {
			var err error
			keys, err = d.sortedKeys(n.keys, mp, idx)
			if err != nil {
				return "", miniscript.P2WSH, err
			}
		}

		var b strings.Builder
		fmt.Fprintf(&b, "multi(%d", n.thresh)
		for _, key := range keys {
			b.WriteByte(',')
			b.WriteString(key.raw)
		}
		b.WriteByte(')')
		return b.String(), miniscript.P2WSH, nil

	default:
		return "", miniscript.P2WSH, errUnsupportedInner
	}
}

// sortedKeys returns the descriptor keys in the BIP67 order of their derived
// compressed public keys at the given multipath and derivation index.
func (d *Descriptor) sortedKeys(keys []*descKey, mp, idx uint32) ([]*descKey,
	error) {

	type derivedKey struct {
		key *descKey
		pub []byte
	}

	derived := make([]derivedKey, len(keys))
	for i, key := range keys {
		pub, err := key.derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		derived[i] = derivedKey{key: key, pub: pub}
	}

	sort.Slice(derived, func(i, j int) bool {
		return bytes.Compare(derived[i].pub, derived[j].pub) < 0
	})

	out := make([]*descKey, len(derived))
	for i, dk := range derived {
		out[i] = dk.key
	}
	return out, nil
}

// planEcdsa returns a lookup that reports an assumed-size ECDSA signature as
// available whenever the assets provide one for the key.
func planEcdsa(assets Assets) func(string) ([]byte, bool) {
	return func(defKey string) ([]byte, bool) {
		if assets.LookupEcdsaSig == nil {
			return nil, false
		}
		if assets.LookupEcdsaSig(defKey) {
			return make([]byte, assumedEcdsaSigLen), true
		}
		return nil, false
	}
}

// realEcdsa returns a lookup backed by the satisfier's ECDSA signatures.
func realEcdsa(s *Satisfier) func(string) ([]byte, bool) {
	return func(defKey string) ([]byte, bool) {
		if s.LookupEcdsaSig != nil {
			return s.LookupEcdsaSig(defKey)
		}
		return nil, false
	}
}

// planTapLeaf returns a lookup that reports an asset-sized taproot leaf-script
// signature as available whenever the assets provide one for the key.
func planTapLeaf(assets Assets, leafHash string) func(string) ([]byte, bool) {
	return func(defKey string) ([]byte, bool) {
		if assets.LookupTapLeafScriptSig == nil {
			return nil, false
		}
		size, ok := assets.LookupTapLeafScriptSig(defKey, leafHash)
		if !ok {
			return nil, false
		}
		return make([]byte, size), true
	}
}

// realTapLeaf returns a lookup backed by the satisfier's taproot leaf-script
// signatures.
func realTapLeaf(s *Satisfier, leafHash string) func(string) ([]byte, bool) {
	return func(defKey string) ([]byte, bool) {
		if s.LookupTapLeafScriptSig != nil {
			return s.LookupTapLeafScriptSig(defKey, leafHash)
		}
		return nil, false
	}
}

// wpkhProgram returns the 20-byte witness program (the HASH160 of the derived
// public key) of a wpkh node.
func (d *Descriptor) wpkhProgram(n *node, mp, idx uint32) ([]byte, error) {
	pubKey, err := n.keys[0].derive(mp, idx, false)
	if err != nil {
		return nil, err
	}
	return address.Hash160(pubKey), nil
}

// witnessSerializedSize returns the serialized size in bytes of a witness
// stack: the var-int element count plus each element's var-int length prefix
// and bytes.
func witnessSerializedSize(w [][]byte) uint64 {
	size := uint64(varintLen(uint64(len(w))))
	for _, element := range w {
		size += uint64(varintLen(uint64(len(element)))) +
			uint64(len(element))
	}
	return size
}

// pushScript returns a script that pushes the given data as a single element.
func pushScript(data []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().AddData(data).Script()
}

// pushAll returns a script that pushes each of the given elements in order.
func pushAll(elements [][]byte) ([]byte, error) {
	b := txscript.NewScriptBuilder()
	for _, element := range elements {
		b.AddData(element)
	}
	return b.Script()
}

// relativeImplied reports whether a required relative locktime is implied by
// the given maximum, following BIP68: both must be of the same unit (blocks or
// time) and the required value must not exceed the maximum.
func relativeImplied(required uint32, max *uint32) bool {
	if max == nil {
		return false
	}

	const (
		typeFlag  = uint32(1) << 22
		valueMask = uint32(0xffff)
	)
	if required&typeFlag != *max&typeFlag {
		return false
	}
	return required&valueMask <= *max&valueMask
}

// absoluteImplied reports whether a required absolute locktime is implied by
// the given maximum, following BIP65: both must be of the same kind (block
// height or unix time) and the required value must not exceed the maximum.
func absoluteImplied(required uint32, max *uint32) bool {
	if max == nil {
		return false
	}

	const heightTimeThreshold = uint32(500000000)
	if (required < heightTimeThreshold) != (*max < heightTimeThreshold) {
		return false
	}
	return required <= *max
}

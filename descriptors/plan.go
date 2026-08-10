package descriptors

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
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

	// maxSchnorrSigLen is the largest size in bytes a BIP341 signature can
	// have: 64 bytes, plus a sighash byte for any sighash type other than
	// the default. A size reported by an asset provider that exceeds it
	// cannot describe a signature, so it is treated as unavailable rather
	// than allocated.
	maxSchnorrSigLen = 65

	// hashPreimageLen is the size in bytes of the preimage of a miniscript
	// hash fragment. Every one of the four fragments checks the size of the
	// element it is satisfied with to be 32 bytes, so a preimage of any
	// other size cannot satisfy one.
	hashPreimageLen = 32

	// emptyScriptSigSize is the serialized size of an empty scriptSig,
	// which is the var-int that prefixes it in the transaction. A segwit
	// spend of a native output has one.
	emptyScriptSigSize = 1
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
// witness template. Any nil lookup is treated as "not available". Public-key
// strings are definite descriptor key expressions: origins are retained,
// multipath choices and wildcards are resolved, and hardened steps use "'".
// Leaf hashes use forward-byte-order hex. Callbacks must not mutate arguments.
type Assets struct {
	// LookupEcdsaSig reports whether an ECDSA signature is available for
	// the given public key.
	LookupEcdsaSig func(pk string) bool

	// LookupTapKeySpendSig reports whether a taproot key-spend signature is
	// available for the given public key, and its size. A size larger than
	// a BIP341 signature (maxSchnorrSigLen) is treated as unavailable.
	LookupTapKeySpendSig func(pk string) (uint32, bool)

	// LookupTapLeafScriptSig reports whether a taproot leaf-script
	// signature is available for the given public key and leaf hash, and
	// its size. A size larger than a BIP341 signature (maxSchnorrSigLen) is
	// treated as unavailable.
	LookupTapLeafScriptSig func(pk string, leafHash string) (uint32, bool)

	// LookupPreimage reports whether the preimage of a hash fragment is
	// available. hashFunc is the name of the fragment, i.e. one of
	// "sha256", "hash256", "ripemd160" and "hash160", and hash the hash
	// value it commits to. A preimage is always 32 bytes, so unlike a
	// signature it needs no size.
	LookupPreimage func(hashFunc string, hash []byte) bool

	// TxVersion is the version of the transaction the spend is planned for.
	// A relative locktime (older) is only enforced by a transaction of
	// version 2 or later (BIP68), so a plan can only rely on one if this
	// field says so.
	TxVersion *int32

	// TxLockTime is the nLockTime of the transaction the spend is planned
	// for, which bounds the absolute locktimes (after) a plan may rely on
	// (BIP65).
	TxLockTime *uint32

	// TxInputSequence is the nSequence of the input being spent, which both
	// kinds of locktime depend on. A relative locktime is the sequence
	// itself, which BIP68 only enforces if its disable flag (bit 31) is
	// clear, and an absolute locktime is only enforced by BIP65 for an
	// input whose sequence is not SEQUENCE_FINAL (0xffffffff). Note that
	// both values wallets commonly default to, 0xffffffff and 0xfffffffd,
	// have the disable flag set, and that the first of them rules out an
	// absolute locktime too.
	TxInputSequence *uint32
}

// checkOlder reports whether the relative locktime of an older(lt) fragment is
// compatible with the transaction these assets describe, which needs
// both its version and the sequence of the input being spent.
// It does not check the age of the spent output against the chain tip.
func (a Assets) checkOlder(lt uint32) bool {
	if a.TxVersion == nil || a.TxInputSequence == nil {
		return false
	}

	return miniscript.CheckOlder(lt, *a.TxVersion, *a.TxInputSequence)
}

// checkAfter reports whether the absolute locktime of an after(lt) fragment is
// compatible with the transaction these assets describe, which needs
// both its locktime and the sequence of the input being spent.
// It does not check transaction finality against the chain tip.
func (a Assets) checkAfter(lt uint32) bool {
	if a.TxLockTime == nil || a.TxInputSequence == nil {
		return false
	}

	return miniscript.CheckAfter(lt, *a.TxLockTime, *a.TxInputSequence)
}

// Satisfier provides signatures and preimages to complete a plan. Nil lookups
// are unavailable. Key and hash identifiers follow the Assets conventions.
type Satisfier struct {
	// LookupEcdsaSig returns a DER-encoded ECDSA signature (including the
	// sighash type byte) for the given public key.
	LookupEcdsaSig func(pk string) ([]byte, bool)

	// LookupTapKeySpendSig returns a taproot key-spend signature.
	LookupTapKeySpendSig func() ([]byte, bool)

	// LookupTapLeafScriptSig returns a taproot leaf-script signature for
	// the given public key and leaf hash.
	LookupTapLeafScriptSig func(pk string, leafHash string) ([]byte, bool)

	// LookupPreimage returns the 32-byte preimage of a hash fragment,
	// keyed like Assets.LookupPreimage.
	LookupPreimage func(hashFunc string, hash []byte) ([]byte, bool)
}

// SatisfyResult is the completed witness and scriptSig produced by a plan.
type SatisfyResult struct {
	// Witness is the witness stack, including the script the spend has to
	// reveal: the witness script of a P2WSH output, or the leaf script and
	// control block of a taproot script path.
	Witness [][]byte

	// ScriptSig is the scriptSig bytes, including the redeem script of a
	// P2SH output.
	ScriptSig []byte
}

// Plan is a chosen spending path on a descriptor: it captures the weight of the
// satisfaction and knows how to complete it from a Satisfier. Obtain it from
// PlanAt; its zero value is not usable. Size methods report planned sizes, not
// the sizes of subsequently supplied signatures.
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
// this plan (both the scriptSig and the witness, including their serialization
// prefixes). It excludes the outpoint, sequence and transaction-wide witness
// marker/flag. ECDSA signatures are estimated at 72 bytes including sighash.
func (p *Plan) SatisfactionWeight() uint64 {
	return p.witnessSize + p.scriptSigSize*4
}

// ScriptSigSize returns the size in bytes of the scriptSig that satisfies this
// plan, including its var-int length prefix. It covers exactly the bytes
// Satisfy produces, so for a P2SH output it counts the redeem script the
// scriptSig has to reveal, which is where it diverges from rust-miniscript's
// plan (see the differential test).
func (p *Plan) ScriptSigSize() uint64 {
	return p.scriptSigSize
}

// WitnessSize returns the size in bytes of the witness that satisfies this
// plan. Like ScriptSigSize, it covers exactly the bytes Satisfy produces, so
// for a P2WSH output it counts the witness script the witness has to reveal,
// which is where it diverges from rust-miniscript's plan (see the differential
// test).
func (p *Plan) WitnessSize() uint64 {
	return p.witnessSize
}

// Satisfy completes the plan, producing the final witness and scriptSig, or an
// error if the satisfier cannot provide the required data. It does not select
// another path if data is missing. Signature lengths may differ from those
// planned, so callers should measure the result before finalizing fees.
// Signatures and preimage hashes are not cryptographically verified, nor is
// the spending transaction checked against the locktimes used during planning.
func (p *Plan) Satisfy(satisfier *Satisfier) (*SatisfyResult, error) {
	// The lookups are read from the satisfier by the closures the plan was
	// built with, which would dereference the nil pointer instead of
	// reporting that nothing can be looked up at all.
	if satisfier == nil {
		return nil, fmt.Errorf("%w: no satisfier provided",
			errCouldNotSatisfy)
	}

	return p.satisfy(satisfier)
}

// PlanAt returns a plan for the given multipath and derivation index if the
// provided assets are sufficient to produce a non-malleable satisfaction.
// The index constraints are the same as for AddressAt. Availability callbacks
// run during planning; completion uses only the chosen path's lookups.
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
	case nodeWsh:
		return d.planWitnessScript(
			n.sub, mp, idx, assets, emptyScriptSigSize, nil,
		)

	case nodeSh:
		return d.planSh(n.sub, mp, idx, assets)

	case nodeWpkh:
		return d.planKeyHash(
			n, mp, idx, assets, true, emptyScriptSigSize, nil,
		)

	case nodePkh:
		return d.planKeyHash(n, mp, idx, assets, false, 0, nil)

	default:
		// A bare script: the whole satisfaction is the scriptSig, and
		// there is no redeem script to reveal.
		return d.planBare(n, mp, idx, assets, nil)
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
			sub.sub, mp, idx, assets,
			scriptSigSerializedSize([][]byte{redeem}), scriptSig,
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
			sub, mp, idx, assets, true,
			scriptSigSerializedSize([][]byte{redeem}), scriptSig,
		)

	default:
		// A legacy P2SH: the redeem script and the satisfaction are
		// both in the scriptSig.
		redeem, err := d.innerScript(sub, mp, idx)
		if err != nil {
			return nil, err
		}

		return d.planBare(sub, mp, idx, assets, redeem)
	}
}

// planWitnessScript builds a plan whose satisfaction is the miniscript
// satisfaction of a witness script (a wsh inner or a P2SH-wrapped wsh inner).
// scriptSig is the (possibly empty) scriptSig; scriptSigSize its accounted
// size.
func (d *Descriptor) planWitnessScript(sub *node, mp, idx uint32, assets Assets,
	scriptSigSize uint64, scriptSig []byte) (*Plan, error) {

	// A P2WSH spend has to reveal the witness script as the last element of
	// its witness (BIP141), so it is part of both the witness the plan
	// produces and the size it reports for it.
	witnessScript, err := d.innerScript(sub, mp, idx)
	if err != nil {
		return nil, err
	}

	template, err := d.satisfyScript(
		sub, mp, idx, assets, planEcdsa(assets), planPreimage(assets),
	)
	if err != nil {
		return nil, errCouldNotPlan
	}
	template = append(template, witnessScript)

	if scriptSig == nil {
		scriptSig = []byte{}
	}

	return &Plan{
		witnessSize:   witnessSerializedSize(template),
		scriptSigSize: scriptSigSize,
		satisfy: func(s *Satisfier) (*SatisfyResult, error) {
			witness, err := d.satisfyScript(
				sub, mp, idx, assets, realEcdsa(s),
				realPreimage(s),
			)
			if err != nil {
				return nil, errCouldNotSatisfy
			}
			return &SatisfyResult{
				Witness:   append(witness, witnessScript),
				ScriptSig: scriptSig,
			}, nil
		},
	}, nil
}

// planBare builds a plan for a bare or legacy-P2SH script, where the entire
// satisfaction lives in the scriptSig. redeem is the redeem script of a P2SH
// output, which the scriptSig has to reveal, or nil for a bare script.
func (d *Descriptor) planBare(sub *node, mp, idx uint32, assets Assets,
	redeem []byte) (*Plan, error) {

	template, err := d.satisfyScript(
		sub, mp, idx, assets, planEcdsa(assets), planPreimage(assets),
	)
	if err != nil {
		return nil, errCouldNotPlan
	}

	return &Plan{
		witnessSize: 0,
		scriptSigSize: scriptSigSerializedSize(
			legacyScriptSig(template, redeem),
		),
		satisfy: func(s *Satisfier) (*SatisfyResult, error) {
			witness, err := d.satisfyScript(
				sub, mp, idx, assets, realEcdsa(s),
				realPreimage(s),
			)
			if err != nil {
				return nil, errCouldNotSatisfy
			}
			scriptSig, err := pushAll(
				legacyScriptSig(witness, redeem),
			)
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

	pubKey, err := key.derive(mp, idx)
	if err != nil {
		return nil, err
	}

	// The satisfaction is <sig> <pubkey>: in the witness of a P2WPKH spend,
	// in the scriptSig of a P2PKH one.
	template := [][]byte{make([]byte, assumedEcdsaSigLen), pubKey}
	witnessSize := witnessSerializedSize(template)
	scriptSigOnlySize := scriptSigSerializedSize(template)

	if scriptSig == nil {
		scriptSig = []byte{}
	}

	plan := &Plan{}
	if segwit {
		plan.witnessSize = witnessSize
		plan.scriptSigSize = scriptSigSize
	} else {
		plan.witnessSize = 0
		plan.scriptSigSize = scriptSigOnlySize
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

// satisfyScript runs the miniscript satisfaction of a wsh inner or tapscript
// leaf node, resolving signatures through the given lookup keyed by the
// definite key string. It returns the witness stack (excluding the witness
// script).
func (d *Descriptor) satisfyScript(n *node, mp, idx uint32, assets Assets,
	lookup func(defKey string) ([]byte, bool),
	preimage miniscript.PreimageFunc) ([][]byte, error) {

	// In a pre-segwit position, the key expressions of pk, pkh, multi and
	// sortedmulti may hold uncompressed keys (BIP381, BIP383), which the
	// miniscript engine cannot represent: miniscript is only defined for
	// the wsh() and tr() contexts and works with compressed keys (BIP379).
	// Their satisfaction is a fixed sequence of signatures, so it is built
	// here instead of going through the engine.
	if n.pos == posTop || n.pos == posSh {
		switch n.kind {
		case nodePk, nodePkh, nodeMulti, nodeSortedMulti:
			return d.satisfyLegacyKeys(n, mp, idx, lookup)
		}
	}

	ast, _, err := d.planAST(n, mp, idx)
	if err != nil {
		return nil, err
	}

	err = ast.ApplyVars(d.lookupKey(mp, idx))
	if err != nil {
		return nil, err
	}

	// Map each derived public key back to the descriptor keys it came from,
	// so the sign callback can resolve the definite key string it was asked
	// to look up.
	//
	// Two key expressions of the same descriptor can derive to the same
	// public key: the same point spelled x-only in one tap leaf and
	// compressed in another is a single 32-byte key in the P2TR context.
	// Every spelling is therefore kept and offered to the lookup in turn,
	// as the caller may hold the signature under any one of them.
	pkMap := make(map[string][]*descKey)
	for _, k := range d.keys {
		pub, err := k.derive(mp, idx)
		if err != nil {
			continue
		}
		pkMap[string(pub)] = append(pkMap[string(pub)], k)
	}

	satisfier := &miniscript.Satisfier{
		Sign: func(pubKey []byte) ([]byte, bool) {
			for _, k := range pkMap[string(pubKey)] {
				sig, ok := lookup(k.definiteString(mp, idx))
				if ok {
					return sig, true
				}
			}
			return nil, false
		},
		CheckOlder: func(lt uint32) (bool, error) {
			return assets.checkOlder(lt), nil
		},
		CheckAfter: func(lt uint32) (bool, error) {
			return assets.checkAfter(lt), nil
		},
		Preimage: preimage,
	}

	return ast.Satisfy(satisfier)
}

// satisfyLegacyKeys builds the satisfaction of a pre-segwit pk, pkh, multi or
// sortedmulti node, which is a fixed sequence of elements in the order the
// script consumes them, resolved through the given signature lookup. It returns
// an error if the lookup cannot provide the signatures the script requires.
func (d *Descriptor) satisfyLegacyKeys(n *node, mp, idx uint32,
	lookup func(defKey string) ([]byte, bool)) ([][]byte, error) {

	switch n.kind {
	case nodePk, nodePkh:
		key := n.keys[0]
		sig, ok := lookup(key.definiteString(mp, idx))
		if !ok {
			return nil, errCouldNotPlan
		}

		// A P2PK spend is just the signature, while a P2PKH spend also
		// reveals the public key the script commits to.
		if n.kind == nodePk {
			return [][]byte{sig}, nil
		}

		pubKey, err := key.derive(mp, idx)
		if err != nil {
			return nil, err
		}

		return [][]byte{sig, pubKey}, nil

	case nodeMulti, nodeSortedMulti:
		keys := n.keys
		if n.kind == nodeSortedMulti {
			var err error
			keys, err = d.sortedKeys(n.keys, mp, idx)
			if err != nil {
				return nil, err
			}
		}

		// Collect the available signatures in key order, which is the
		// order OP_CHECKMULTISIG requires them in.
		var sigs [][]byte
		for _, key := range keys {
			sig, ok := lookup(key.definiteString(mp, idx))
			if ok {
				sigs = append(sigs, sig)
			}
		}
		if len(sigs) < n.thresh {
			return nil, errCouldNotPlan
		}

		// Only the threshold many signatures are needed. Drop the
		// largest ones, keeping the rest in key order, so that the
		// satisfaction is as small as possible, matching what the
		// miniscript satisfier does for the same script.
		for len(sigs) > n.thresh {
			maxIdx := 0
			for i := range sigs {
				if len(sigs[i]) > len(sigs[maxIdx]) {
					maxIdx = i
				}
			}
			sigs = slices.Delete(sigs, maxIdx, maxIdx+1)
		}

		// The leading empty element is the dummy that the
		// OP_CHECKMULTISIG off-by-one bug consumes.
		return append([][]byte{{}}, sigs...), nil

	default:
		return nil, errUnsupportedInner
	}
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
// serialized public keys at the given multipath and derivation index. It leaves
// the input slice unchanged and preserves legacy uncompressed serializations.
func (d *Descriptor) sortedKeys(keys []*descKey, mp, idx uint32) ([]*descKey,
	error) {

	type derivedKey struct {
		key *descKey
		pub []byte
	}

	derived := make([]derivedKey, len(keys))
	for i, key := range keys {
		pub, err := key.derive(mp, idx)
		if err != nil {
			return nil, err
		}
		derived[i] = derivedKey{key: key, pub: pub}
	}

	slices.SortFunc(derived, func(a, b derivedKey) int {
		return bytes.Compare(a.pub, b.pub)
	})

	out := make([]*descKey, len(derived))
	for i, dk := range derived {
		out[i] = dk.key
	}
	return out, nil
}

// planPreimage returns a lookup that reports a dummy preimage as available
// whenever the assets provide one for the hash value of a fragment. A preimage
// is 32 bytes whatever it hashes to, so the dummy has the size of the real one.
func planPreimage(assets Assets) miniscript.PreimageFunc {
	return func(hashFunc string, hash []byte) ([]byte, bool) {
		if assets.LookupPreimage == nil {
			return nil, false
		}
		if !assets.LookupPreimage(hashFunc, hash) {
			return nil, false
		}

		return make([]byte, hashPreimageLen), true
	}
}

// realPreimage returns a lookup backed by the satisfier's preimages.
func realPreimage(s *Satisfier) miniscript.PreimageFunc {
	return func(hashFunc string, hash []byte) ([]byte, bool) {
		if s.LookupPreimage == nil {
			return nil, false
		}

		return s.LookupPreimage(hashFunc, hash)
	}
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

// wpkhProgram returns the 20-byte witness program (the HASH160 of the derived
// public key) of a wpkh node.
func (d *Descriptor) wpkhProgram(n *node, mp, idx uint32) ([]byte, error) {
	pubKey, err := n.keys[0].derive(mp, idx)
	if err != nil {
		return nil, err
	}
	return address.Hash160(pubKey), nil
}

// scriptSigSerializedSize returns the serialized size in bytes of a scriptSig
// that pushes the given elements, including the var-int that prefixes the
// scriptSig in the transaction. Unlike a witness, a scriptSig has no element
// count and prefixes every element with a push opcode instead of a var-int.
func scriptSigSerializedSize(elements [][]byte) uint64 {
	size := 0
	for _, element := range elements {
		size += pushOpcodeSize(len(element)) + len(element)
	}

	return uint64(varintLen(uint64(size)) + size)
}

// legacyScriptSig returns the elements a legacy scriptSig pushes: the
// satisfaction, followed by the redeem script if the output being spent is a
// P2SH. The redeem script has to be part of the scriptSig, otherwise the spend
// does not even reveal the script it is supposed to satisfy.
func legacyScriptSig(satisfaction [][]byte, redeem []byte) [][]byte {
	if redeem == nil {
		return satisfaction
	}

	// Copy the outer slice so appending the redeem script cannot overwrite
	// spare capacity in the caller's satisfaction. Element bytes are
	// shared.
	return append(slices.Clone(satisfaction), redeem)
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

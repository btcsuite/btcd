package descriptors

import (
	"errors"
)

// errUnsupportedInner is returned when a descriptor node's weight cannot be
// estimated because its inner script kind is not supported.
var errUnsupportedInner = errors.New(
	"unsupported inner script for weight estimation",
)

// Signature, key and control-block sizes assumed when estimating a
// satisfaction, matching rust-miniscript. All ECDSA signatures are assumed
// to be 73 bytes and all Schnorr signatures 66 bytes, in both cases including
// the length prefix and sighash byte. A compressed public key push is 34
// bytes.
const (
	// ecdsaSigSize is the assumed size of an ECDSA signature push.
	ecdsaSigSize = 73

	// compressedKeyPush is the size of a compressed public key push (33
	// bytes plus a one-byte length prefix).
	compressedKeyPush = 34

	// schnorrSigPush is the assumed size of a Schnorr key-spend signature
	// push (a 64-byte signature plus a one-byte sighash byte and a one-byte
	// length prefix).
	schnorrKeySpendSig = 1 + 65

	// tapControlBlockBase is the size of a taproot control block with an
	// empty Merkle path (a single-leaf script tree): the leaf version byte
	// plus the 32-byte internal key.
	tapControlBlockBase = 33

	// tapControlBlockNode is the size of one Merkle path element added to a
	// control block per level of tree depth.
	tapControlBlockNode = 32
)

// MaxWeightToSatisfy returns an upper bound on the weight, in weight units, of
// the input (witness and/or scriptSig) needed to satisfy this descriptor. It
// mirrors rust-miniscript's max_weight_to_satisfy and returns an error if the
// descriptor cannot be satisfied.
func (d *Descriptor) MaxWeightToSatisfy() (uint64, error) {
	return d.maxWeight(d.root)
}

// maxWeight dispatches the weight computation on the descriptor's top-level
// output type.
func (d *Descriptor) maxWeight(n *node) (uint64, error) {
	switch n.kind {
	case nodeWpkh:
		// Witness: <sig> <pubkey>, two stack items.
		stackItems := ecdsaSigSize + compressedKeyPush
		stackVarintDiff := varintLen(2) - varintLen(0)
		return uint64(stackVarintDiff + stackItems), nil

	case nodePkh:
		// Legacy scriptSig: <sig> <pubkey>.
		scriptSig := ecdsaSigSize + compressedKeyPush
		scriptSigVarintDiff := varintLen(uint64(scriptSig)) -
			varintLen(0)
		return weightVB(scriptSigVarintDiff + scriptSig), nil

	case nodeTr:
		return d.maxWeightTr(n)

	case nodeWsh:
		return d.maxWeightWsh(n.sub)

	case nodeSh:
		return d.maxWeightSh(n.sub)

	default:
		// A bare descriptor: a top-level pk(), a bare multisig, or a
		// bare miniscript. Its satisfaction lives entirely in the
		// scriptSig.
		_, satSize, _, err := d.satInfo(n)
		if err != nil {
			return 0, err
		}
		scriptSigVarintDiff := varintLen(uint64(satSize)) - varintLen(0)
		return weightVB(scriptSigVarintDiff + satSize), nil
	}
}

// maxWeightWsh computes the witness weight of a P2WSH output whose witness
// script is the given inner node.
func (d *Descriptor) maxWeightWsh(sub *node) (uint64, error) {
	scriptSize, satSize, satElems, err := d.satInfo(sub)
	if err != nil {
		return 0, err
	}

	// The witness stack holds the satisfaction elements plus the witness
	// script itself (already counted in satElems).
	stackVarintDiff := varintLen(uint64(satElems)) - varintLen(0)
	wu := stackVarintDiff + varintLen(uint64(scriptSize)) + scriptSize +
		satSize

	return uint64(wu), nil
}

// maxWeightSh computes the weight of a P2SH output whose redeem script is the
// given inner node, which may itself be a P2WSH, P2WPKH or a legacy script.
func (d *Descriptor) maxWeightSh(sub *node) (uint64, error) {
	var (
		scriptSig     int
		witnessWeight uint64
	)

	switch sub.kind {
	case nodeWsh:
		// scriptSig: OP_34 <OP_0 OP_32 <32-byte hash>>.
		scriptSig = 1 + 1 + 1 + 32
		w, err := d.maxWeightWsh(sub.sub)
		if err != nil {
			return 0, err
		}
		witnessWeight = w

	case nodeWpkh:
		// scriptSig: OP_22 <OP_0 OP_20 <20-byte hash>>.
		scriptSig = 1 + 1 + 1 + 20
		w, err := d.maxWeight(sub)
		if err != nil {
			return 0, err
		}
		witnessWeight = w

	default:
		// A legacy P2SH whose satisfaction, redeem script and its push
		// opcode all live in the scriptSig.
		scriptSize, satSize, _, err := d.satInfo(sub)
		if err != nil {
			return 0, err
		}
		scriptSig = pushOpcodeSize(scriptSize) + scriptSize + satSize
	}

	scriptSigVarintDiff := varintLen(uint64(scriptSig)) - varintLen(0)

	return weightVB(scriptSigVarintDiff+scriptSig) + witnessWeight, nil
}

// maxWeightTr computes the witness weight of a P2TR output. For a
// key-path-only output this is a single Schnorr signature; for an output with a
// script tree it is the largest script-path spend over all leaves, matching
// rust-miniscript which always accounts for the script path when a tap tree is
// present.
func (d *Descriptor) maxWeightTr(n *node) (uint64, error) {
	if n.tapTree == nil {
		// Key spend: a single Schnorr signature element.
		stackVarintDiff := varintLen(1) - varintLen(0)
		return uint64(stackVarintDiff + schnorrKeySpendSig), nil
	}

	var maxWU uint64
	err := n.tapTree.forEachLeaf(0, func(leaf *node, depth int) error {
		scriptSize, satSize, satElems, err := d.satInfo(leaf)
		if err != nil {
			return err
		}

		// The control block grows by one 32-byte hash per level of
		// depth (an empty merkle path for a single-leaf tree).
		controlBlock := tapControlBlockBase +
			depth*tapControlBlockNode

		// The witness stack additionally holds the leaf script and the
		// control block; satElems already includes the leaf script, so
		// only the control block adds to the element count (hence +1).
		stackVarintDiff := varintLen(uint64(satElems+1)) - varintLen(0)
		wu := uint64(stackVarintDiff + satSize +
			varintLen(uint64(scriptSize)) + scriptSize +
			varintLen(uint64(controlBlock)) + controlBlock)

		if wu > maxWU {
			maxWU = wu
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return maxWU, nil
}

// satInfo returns the script size, the maximum satisfaction witness byte size
// and the maximum satisfaction witness element count (including the script) of
// an inner descriptor node (the inner of a wsh/sh, a taproot leaf, or a bare
// script). It returns an error if the node cannot be satisfied.
func (d *Descriptor) satInfo(n *node) (scriptSize, satSize, satElems int,
	err error) {

	switch n.kind {
	case nodeMs:
		ast := n.clonedMsAST()
		satSize, err := ast.MaxSatisfactionSize()
		if err != nil {
			return 0, 0, 0, err
		}
		satElems, err := ast.MaxSatisfactionWitnessElements()
		if err != nil {
			return 0, 0, 0, err
		}
		return ast.ScriptLen(), satSize, satElems, nil

	case nodeMulti, nodeSortedMulti:
		k := n.thresh
		numKeys := len(n.keys)
		scriptSize = multiNumCost(k, numKeys) +
			compressedKeyPush*numKeys + 1

		// Satisfaction: a leading OP_0 (one empty element) plus k
		// signatures. The witness element count is k+1, and the wsh
		// element count additionally includes the witness script.
		satSize = 1 + ecdsaSigSize*k
		satElems = (k + 1) + 1
		return scriptSize, satSize, satElems, nil

	case nodePk:
		// A bare or wsh/sh pk() compiles to c:pk_k: <key> CHECKSIG.
		scriptSize = compressedKeyPush + 1
		satSize = ecdsaSigSize
		satElems = 1 + 1
		return scriptSize, satSize, satElems, nil

	case nodePkh:
		// A wsh/sh pkh() compiles to c:pk_h: DUP HASH160 <hash>
		// EQUALVERIFY CHECKSIG.
		scriptSize = 25
		satSize = compressedKeyPush + ecdsaSigSize
		satElems = 2 + 1
		return scriptSize, satSize, satElems, nil

	default:
		return 0, 0, 0, errUnsupportedInner
	}
}

// multiNumCost returns the number of bytes used to encode the two threshold
// pushes (k and n) of a multisig script.
func multiNumCost(k, n int) int {
	switch {
	case k > 16 && n > 16:
		return 4

	case k > 16 || n > 16:
		return 3

	default:
		return 2
	}
}

// varintLen returns the number of bytes used to encode n as a Bitcoin variable
// length integer. It takes a uint64 (the domain of a Bitcoin var-int) so the
// size boundaries are representable on 32-bit platforms, where int is 32 bits.
func varintLen(n uint64) int {
	switch {
	case n < 0xfd:
		return 1

	case n <= 0xffff:
		return 3

	case n <= 0xffffffff:
		return 5

	default:
		return 9
	}
}

// pushOpcodeSize returns the number of bytes used by the opcode that pushes a
// script of the given size onto the stack.
func pushOpcodeSize(scriptSize int) int {
	switch {
	case scriptSize < 76:
		return 1

	case scriptSize < 0x100:
		return 2

	case scriptSize < 0x10000:
		return 3

	default:
		return 5
	}
}

// weightVB converts a virtual byte count to weight units.
func weightVB(vb int) uint64 {
	return uint64(vb) * 4
}

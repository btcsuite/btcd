package miniscript

import (
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// timelockInfo tracks which kinds of time locks may be encountered while
// satisfying a (sub)expression. Absolute time locks come from the `after`
// fragment (OP_CHECKLOCKTIMEVERIFY), relative time locks from the `older`
// fragment (OP_CHECKSEQUENCEVERIFY). Each is either expressed in block heights
// or in units of time.
//
// It is a port of rust-miniscript's `TimelockInfo` and is used to detect time
// lock mixing: an expression that requires both a height-based and a time-based
// lock of the same kind on a single spending path has a branch that can never
// be satisfied, see
// https://medium.com/blockstream/dont-mix-your-timelocks-d9939b665094.
type timelockInfo struct {
	// csvWithHeight is set if the expression contains a relative,
	// block-height-based time lock (`older` with the type flag unset).
	csvWithHeight bool

	// csvWithTime is set if the expression contains a relative, time-based
	// time lock (`older` with the type flag set).
	csvWithTime bool

	// cltvWithHeight is set if the expression contains an absolute,
	// block-height-based time lock (`after` with a value below the
	// threshold).
	cltvWithHeight bool

	// cltvWithTime is set if the expression contains an absolute,
	// time-based time lock (`after` with a value at or above the
	// threshold).
	cltvWithTime bool

	// containsCombination is set if some spending path within the
	// expression mixes a height-based and a time-based lock of the same
	// kind, which makes that path unspendable.
	containsCombination bool
}

// combineTimelocks folds the time lock info of the sub expressions of a
// fragment into a single value. If more than one of the sub expressions can be
// required simultaneously (k > 1, i.e. a conjunction), height and time locks of
// the same kind that end up on the same spending path are flagged in
// containsCombination.
//
// This is a port of rust-miniscript's `TimelockInfo::combine_threshold`.
func combineTimelocks(k int, subs ...timelockInfo) timelockInfo {
	var acc timelockInfo
	for _, t := range subs {
		// If more than one branch may be taken at once, and this branch
		// has a requirement that conflicts with one already
		// accumulated, the combined path is unspendable.
		if k > 1 {
			heightAndTime := (acc.csvWithHeight && t.csvWithTime) ||
				(acc.csvWithTime && t.csvWithHeight) ||
				(acc.cltvWithTime && t.cltvWithHeight) ||
				(acc.cltvWithHeight && t.cltvWithTime)

			acc.containsCombination =
				acc.containsCombination || heightAndTime
		}
		acc.csvWithHeight = acc.csvWithHeight || t.csvWithHeight
		acc.csvWithTime = acc.csvWithTime || t.csvWithTime
		acc.cltvWithHeight = acc.cltvWithHeight || t.cltvWithHeight
		acc.cltvWithTime = acc.cltvWithTime || t.cltvWithTime
		acc.containsCombination =
			acc.containsCombination || t.containsCombination
	}

	return acc
}

// combineTimelocksAnd combines the time lock info of two sub expressions that
// are both required (logical and).
func combineTimelocksAnd(a, b timelockInfo) timelockInfo {
	return combineTimelocks(2, a, b)
}

// combineTimelocksOr combines the time lock info of two sub expressions of
// which only one is required (logical or).
func combineTimelocksOr(a, b timelockInfo) timelockInfo {
	return combineTimelocks(1, a, b)
}

// computeTimelocks computes the timelockInfo of a node from that of its
// children. It is applied bottom-up as part of Parse.
func computeTimelocks(node *AST) (*AST, error) {
	switch node.identifier {
	case f_after:
		// Absolute time lock (OP_CHECKLOCKTIMEVERIFY, BIP65). Values
		// below the threshold are block heights, values at or above it
		// are Unix time stamps.
		n := node.args[0].num
		node.timelock = timelockInfo{
			cltvWithHeight: n < uint64(txscript.LockTimeThreshold),
			cltvWithTime:   n >= uint64(txscript.LockTimeThreshold),
		}

	case f_older:
		// Relative time lock (OP_CHECKSEQUENCEVERIFY, BIP68). The type
		// flag (bit 22) selects time-based over height-based locks.
		n := node.args[0].num
		isTime := n&uint64(wire.SequenceLockTimeIsSeconds) != 0
		node.timelock = timelockInfo{
			csvWithHeight: !isTime,
			csvWithTime:   isTime,
		}

	case f_and_v, f_and_b:
		node.timelock = combineTimelocksAnd(
			node.args[0].timelock, node.args[1].timelock,
		)

	case f_or_b, f_or_c, f_or_d, f_or_i:
		node.timelock = combineTimelocksOr(
			node.args[0].timelock, node.args[1].timelock,
		)

	case f_andor:
		// andor(X, Y, Z) is or(and(X, Y), Z).
		node.timelock = combineTimelocksOr(
			combineTimelocksAnd(
				node.args[0].timelock, node.args[1].timelock,
			),
			node.args[2].timelock,
		)

	case f_thresh:
		k := int(node.args[0].num)
		subs := make([]timelockInfo, 0, len(node.args)-1)
		for _, arg := range node.args[1:] {
			subs = append(subs, arg.timelock)
		}
		node.timelock = combineTimelocks(k, subs...)

	case f_wrap_a, f_wrap_s, f_wrap_c, f_wrap_d, f_wrap_v, f_wrap_j,
		f_wrap_n:

		// Wrappers do not change the time lock semantics of their
		// child.
		node.timelock = node.args[0].timelock

	default:
		// The remaining leaves (0, 1, pk_k, pk_h, multi and the hash
		// fragments) contain no time locks.
		node.timelock = timelockInfo{}
	}

	return node, nil
}

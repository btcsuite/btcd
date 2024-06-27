package miniscript

import "fmt"

type maxInt struct {
	valid bool
	value int
}

func (m maxInt) and(b maxInt) maxInt {
	if !m.valid || !b.valid {
		return maxInt{}
	}
	return maxInt{
		valid: true,
		value: m.value + b.value,
	}
}

func (m maxInt) or(b maxInt) maxInt {
	if !m.valid {
		return b
	}
	if !b.valid {
		return m
	}
	if m.value >= b.value {
		return maxInt{
			valid: true,
			value: m.value,
		}
	}
	return maxInt{
		valid: true,
		value: b.value,
	}
}

type ops struct {
	// count is the number of non-push opcodes.
	count int

	// dsat is the number of keys in possibly executed
	// OP_CHECKMULTISIG(VERIFY)s to dissatisfy.
	dsat maxInt

	// sat is the number of keys in possibly executed
	// OP_CHECKMULTISIG(VERIFY)s to satisfy.
	sat maxInt
}

func computeOpCount(node *AST) (*AST, error) {
	zero := maxInt{valid: true, value: 0}
	invalid := maxInt{valid: false}
	switch node.identifier {
	case f_0:
		node.opCount = ops{0, zero, invalid}

	case f_1:
		node.opCount = ops{0, invalid, zero}

	case f_pk_k:
		node.opCount = ops{0, zero, zero}

	case f_pk_h:
		node.opCount = ops{3, zero, zero}

	case f_older, f_after:
		node.opCount = ops{1, invalid, zero}

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		node.opCount = ops{4, zero, zero}

	case f_andor:
		x, y, z := node.args[0], node.args[1], node.args[2]
		node.opCount = ops{
			3 + x.opCount.count + y.opCount.count + z.opCount.count,
			z.opCount.dsat.and(x.opCount.dsat),
			y.opCount.sat.and(x.opCount.sat).or(
				z.opCount.sat.and(x.opCount.dsat),
			),
		}

	case f_and_v:
		x, y := node.args[0], node.args[1]
		node.opCount = ops{
			x.opCount.count + y.opCount.count,
			invalid,
			y.opCount.sat.and(x.opCount.sat),
		}

	case f_and_b:
		x, y := node.args[0], node.args[1]
		node.opCount = ops{
			1 + x.opCount.count + y.opCount.count,
			y.opCount.dsat.and(x.opCount.dsat),
			y.opCount.sat.and(x.opCount.sat),
		}

	case f_or_b:
		x, z := node.args[0], node.args[1]
		node.opCount = ops{
			1 + x.opCount.count + z.opCount.count,
			z.opCount.dsat.and(x.opCount.dsat),
			z.opCount.dsat.and(x.opCount.sat).or(
				z.opCount.sat.and(x.opCount.dsat),
			),
		}

	case f_or_c:
		x, z := node.args[0], node.args[1]
		node.opCount = ops{
			2 + x.opCount.count + z.opCount.count,
			invalid,
			x.opCount.sat.or(z.opCount.sat.and(x.opCount.dsat)),
		}

	case f_or_d:
		x, z := node.args[0], node.args[1]
		node.opCount = ops{
			3 + x.opCount.count + z.opCount.count,
			z.opCount.dsat.and(x.opCount.dsat),
			x.opCount.sat.or(z.opCount.sat.and(x.opCount.dsat)),
		}

	case f_or_i:
		x, z := node.args[0], node.args[1]
		node.opCount = ops{
			3 + x.opCount.count + z.opCount.count,
			x.opCount.dsat.or(z.opCount.dsat),
			x.opCount.sat.or(z.opCount.sat),
		}

	case f_thresh:
		k := node.args[0].num
		n := len(node.args) - 1

		count := 0
		dsat := invalid
		sat := invalid
		for _, arg := range node.args[1:] {
			count += arg.opCount.count + 1
			dsat = dsat.and(arg.opCount.dsat)
		}
		for _, subset := range subsets(n, int(k)) {
			candidateOps := zero
			for i, arg := range node.args[1:] {
				if containsInt(subset, i) {
					candidateOps = arg.opCount.sat.and(
						candidateOps,
					)
				} else {
					candidateOps = arg.opCount.dsat.and(
						candidateOps,
					)
				}
			}
			sat = sat.or(candidateOps)
		}
		node.opCount = ops{count, dsat, sat}

	case f_multi:
		n := len(node.args) - 1
		node.opCount = ops{
			1,
			maxInt{valid: true, value: n},
			maxInt{valid: true, value: n},
		}

	case f_wrap_a:
		x := node.args[0]
		node.opCount = ops{
			2 + x.opCount.count,
			x.opCount.dsat,
			x.opCount.sat,
		}

	case f_wrap_s, f_wrap_c, f_wrap_n:
		x := node.args[0]
		node.opCount = ops{
			1 + x.opCount.count,
			x.opCount.dsat, x.opCount.sat,
		}

	case f_wrap_d:
		x := node.args[0]
		node.opCount = ops{
			3 + x.opCount.count,
			zero, x.opCount.sat,
		}

	case f_wrap_v:
		x := node.args[0]
		opVerify := 0
		if !node.args[0].props.canCollapseVerify {
			opVerify = 1
		}
		node.opCount = ops{
			opVerify + x.opCount.count, invalid, x.opCount.sat,
		}

	case f_wrap_j:
		x := node.args[0]
		node.opCount = ops{4 + x.opCount.count, zero, x.opCount.sat}

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

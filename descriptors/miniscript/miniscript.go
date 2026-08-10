package miniscript

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/txscript/v2"
)

const (
	// compressedPubKeyLen is the length of a compressed public key, as used
	// in the P2WSH context.
	compressedPubKeyLen = 33

	// xOnlyPubKeyLen is the length of an x-only public key, as used in the
	// P2TR (Tapscript) context.
	xOnlyPubKeyLen = 32

	// maxStandardP2WSHScriptSize is the maximum size in bytes of a standard
	// witnessScript in the P2WSH context.
	maxStandardP2WSHScriptSize = 3600

	// maxTapscriptSize is the maximum size in bytes we allow for a
	// Tapscript leaf script.
	//
	// Tapscript itself removes the P2WSH script size limit (a leaf is only
	// bounded by the block weight), but the script builder used by Script()
	// refuses to grow a script beyond txscript.MaxScriptSize. A larger
	// limit here would therefore only accept expressions that can never be
	// compiled, turning a clear parse-time error into a failure at
	// address-derivation, planning or signing time, so the parse limit is
	// the size we can actually emit.
	maxTapscriptSize = txscript.MaxScriptSize

	// maxRedeemScriptSize is the maximum size in bytes of the redeem script
	// of a P2SH output. The redeem script is pushed as a single data
	// element in the spending scriptSig, so the consensus limit on the size
	// of a script element applies to it: an output whose redeem script is
	// larger can never be spent.
	maxRedeemScriptSize = 520

	// multisigMaxKeys is the maximum number of keys in a P2WSH multisig
	// (OP_CHECKMULTISIG).
	multisigMaxKeys = 20

	// checkSigAddMaxKeys is the maximum number of keys in a Tapscript
	// multi_a (OP_CHECKSIGADD) expression.
	checkSigAddMaxKeys = 999
)

const (
	// All fragment identifiers.

	f_0         = "0"         // 0
	f_1         = "1"         // 1
	f_pk_k      = "pk_k"      // pk_k(key)
	f_pk_h      = "pk_h"      // pk_h(key)
	f_pk        = "pk"        // pk(key) = c:pk_k(key)
	f_pkh       = "pkh"       // pkh(key) = c:pk_h(key)
	f_sha256    = "sha256"    // sha256(h)
	f_ripemd160 = "ripemd160" // ripemd160(h)
	f_hash256   = "hash256"   // hash256(h)
	f_hash160   = "hash160"   // hash160(h)
	f_older     = "older"     // older(n)
	f_after     = "after"     // after(n)
	f_andor     = "andor"     // andor(X,Y,Z)
	f_and_v     = "and_v"     // and_v(X,Y)
	f_and_b     = "and_b"     // and_b(X,Y)
	f_and_n     = "and_n"     // and_n(X,Y) = andor(X,Y,0)
	f_or_b      = "or_b"      // or_b(X,Z)
	f_or_c      = "or_c"      // or_c(X,Z)
	f_or_d      = "or_d"      // or_d(X,Z)
	f_or_i      = "or_i"      // or_i(X,Z)
	f_thresh    = "thresh"    // thresh(k,X1,...,Xn)
	f_multi     = "multi"     // multi(k,key1,...,keyn), P2WSH only
	f_multi_a   = "multi_a"   // multi_a(k,key1,...,keyn), P2TR only

	// sortedmulti_a(k,key1,...,keyn) is a multi_a whose keys are sorted by
	// their serialization before the script is built (BIP387). It is P2TR
	// only, like multi_a, and deSugar turns it into a multi_a that carries
	// the sortedKeys flag.
	f_sortedmulti_a = "sortedmulti_a"
	f_wrap_a        = "a" // a:X
	f_wrap_s        = "s" // s:X
	f_wrap_c        = "c" // c:X
	f_wrap_d        = "d" // d:X
	f_wrap_v        = "v" // v:X
	f_wrap_j        = "j" // j:X
	f_wrap_n        = "n" // n:X
	f_wrap_t        = "t" // t:X = and_v(X,1)
	f_wrap_l        = "l" // l:X = or_i(0,X)
	f_wrap_u        = "u" // u:X = or_i(X,0)
)

// Context is the script context a miniscript is used in. It determines the
// allowed fragments, the public key encoding and the resource limits that
// apply.
type Context uint8

const (
	// P2WSH is the pay-to-witness-script-hash (SegWit v0) context. It uses
	// 33-byte compressed public keys, ECDSA signatures and the `multi`
	// fragment (OP_CHECKMULTISIG).
	P2WSH Context = iota

	// P2TR is the pay-to-taproot (Tapscript) context. It uses 32-byte
	// x-only public keys, Schnorr signatures and the `multi_a` fragment
	// (OP_CHECKSIGADD).
	P2TR

	// Legacy is the pre-segwit context, i.e. the redeem script of a
	// pay-to-script-hash output. Like P2WSH it uses 33-byte compressed
	// public keys, ECDSA signatures and the `multi` fragment, but its
	// resource limits are those of a redeem script and its satisfaction,
	// which live in the scriptSig rather than in a witness: the script is
	// limited to 520 bytes and the satisfaction to 1650 bytes.
	Legacy
)

// String returns the human-readable name of the context.
func (c Context) String() string {
	switch c {
	case P2WSH:
		return "P2WSH"

	case P2TR:
		return "P2TR"

	case Legacy:
		return "Legacy"

	default:
		return fmt.Sprintf("unknown context %d", uint8(c))
	}
}

// keyLen returns the expected serialized length of a public key in this
// context: 33 bytes (compressed) for P2WSH and 32 bytes (x-only) for P2TR.
func (c Context) keyLen() int {
	if c == P2TR {
		return xOnlyPubKeyLen
	}

	return compressedPubKeyLen
}

// maxScriptSize returns the maximum allowed script size in bytes for this
// context.
func (c Context) maxScriptSize() int {
	switch c {
	case P2TR:
		return maxTapscriptSize

	case Legacy:
		return maxRedeemScriptSize

	default:
		return maxStandardP2WSHScriptSize
	}
}

// maxMultiKeys returns the maximum number of keys allowed in the context's
// multisig fragment (`multi` for P2WSH, `multi_a` for P2TR).
func (c Context) maxMultiKeys() int {
	if c == P2TR {
		return checkSigAddMaxKeys
	}

	return multisigMaxKeys
}

// ParseInsane parses a miniscript expression like Parse, but without checking
// that the result is sane.
//
// The returned expression may be unspendable, malleable by third parties, spend
// paths may be unsatisfiable due to mixed time locks, and its script or
// satisfaction may violate the resource limits of the context, all of which
// makes it unsafe to derive an address for. Callers that do not specifically
// want to inspect such an expression should use Parse instead, and callers that
// do should validate what they care about through IsSane, IsValidTopLevel or
// the computed properties.
//
// The following transformations are applied to the AST in order:
//  1. argCheck: Checks that the nodes have the correct number of arguments.
func ParseInsane(miniscript string, ctx Context) (*AST, error) {
	node, err := createAST(miniscript, ctx)
	if err != nil {
		return nil, err
	}

	transformers := []func(*AST) (*AST, error){
		argCheck,
	}
	for _, transform := range transformers {
		node, err = node.apply(transform)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// AST is the abstract syntax tree representing a miniscript expression.
type AST struct {
	// ctx is the script context (P2WSH or P2TR) the expression is parsed
	// in. It is the same for every node of a tree.
	ctx Context

	wrappers   string
	identifier string

	// num is the parsed integer for when identifier is expected to be a
	// number, i.e. the first argument of older/after/multi/thresh. This is
	// not used otherwise.
	num uint64

	args []*AST
}

// isSubExpression returns whether the argument at the given index is a
// miniscript sub expression, as opposed to a key/hash variable or the numeric
// argument of older/after/multi/thresh. Only sub expressions are visited by the
// recursive tree passes, and only they count towards the nesting depth.
func (a *AST) isSubExpression(i int) bool {
	switch a.identifier {
	case f_pk_k, f_pk_h, f_pk, f_pkh,
		f_sha256, f_hash256, f_ripemd160, f_hash160,
		f_older, f_after, f_multi, f_multi_a, f_sortedmulti_a:

		// None of the arguments of these functions are miniscript
		// subexpressions - they are variables (or concrete assignments)
		// or numbers.
		return false

	case f_thresh:
		// First argument is a number. The other arguments are
		// subexpressions, which we want to visit, so only skip the
		// first argument.
		return i > 0
	}

	return true
}

// apply transforms subexpressions bottom-up, replacing each child before its
// parent is visited. Value arguments are left untouched for the parent's pass
// to interpret. A failed pass may leave earlier children transformed.
func (a *AST) apply(f func(*AST) (*AST, error)) (*AST, error) {
	for i, arg := range a.args {
		// We don't recurse into arguments which are not miniscript
		// subexpressions themselves: key/hash variables and the numeric
		// arguments of older/after/multi/thresh.
		if !a.isSubExpression(i) {
			continue
		}

		newArg, err := arg.apply(f)
		if err != nil {
			return nil, err
		}
		a.args[i] = newArg
	}

	// Parent analysis depends on the already-transformed child properties.
	return f(a)
}

// stack holds unfinished expressions while the iterative parser attaches
// completed arguments to their parents.
type stack struct {
	elements []*AST
}

// push makes element the current unfinished expression.
func (s *stack) push(element *AST) {
	s.elements = append(s.elements, element)
}

// pop removes the current expression, or returns nil for an empty stack.
func (s *stack) pop() *AST {
	if len(s.elements) == 0 {
		return nil
	}

	top := s.elements[len(s.elements)-1]
	s.elements = s.elements[:len(s.elements)-1]

	return top
}

// top returns the current expression without removing it, or nil if empty.
func (s *stack) top() *AST {
	if len(s.elements) == 0 {
		return nil
	}

	return s.elements[len(s.elements)-1]
}

// size returns the number of unfinished expressions.
func (s *stack) size() int {
	return len(s.elements)
}

// splitString keeps separators as individual slice elements and splits a string
// into a slice of strings based on multiple separators. It removes any empty
// elements from the output slice. The predicate must only match ASCII
// separators, since each matching separator occupies one byte.
func splitString(s string, isSeparator func(c rune) bool) []string {
	// Pre-size the result slice to avoid repeatedly reallocating its
	// backing array as it grows. Each separator becomes its own element and
	// may additionally be preceded by a substring, so the output holds at
	// most 2*(#separators)+1 elements.
	separators := 0
	for _, c := range s {
		if isSeparator(c) {
			separators++
		}
	}
	substrings := make([]string, 0, 2*separators+1)

	// Retain delimiters as tokens so createAST can distinguish argument
	// boundaries from completed expressions, including adjacent delimiters.
	i := 0
	for i < len(s) {
		// Find the index of the first separator in the string.
		j := strings.IndexFunc(s[i:], isSeparator)
		if j == -1 {
			// The remaining bytes form the last identifier token.
			substrings = append(substrings, s[i:])

			return substrings
		}
		j += i

		// Adjacent delimiters must not introduce empty identifier
		// tokens.
		if j > i {
			substrings = append(substrings, s[i:j])
		}

		substrings = append(substrings, s[j:j+1])
		i = j + 1
	}

	return substrings
}

// createAST parses expression structure without recursively descending into
// untrusted input. It retains wrappers for a later pass and leaves fragment
// names, argument counts and value types for argCheck to validate.
func createAST(miniscript string, ctx Context) (*AST, error) {
	// Preserve punctuation so malformed adjacency is rejected rather than
	// normalized into an apparently valid expression.
	tokens := splitString(miniscript, func(c rune) bool {
		return c == '(' || c == ')' || c == ','
	})

	if len(tokens) > 0 {
		first, last := tokens[0], tokens[len(tokens)-1]
		if first == "(" || first == ")" || first == "," ||
			last == "(" || last == "," {

			return nil, errors.New("invalid first or last " +
				"character")
		}
	}

	// Build abstract syntax tree. The parser stack never holds more entries
	// than there are tokens, so we pre-size it to avoid growth
	// reallocations.
	stack := stack{elements: make([]*AST, 0, len(tokens))}
	for i, token := range tokens {
		switch token {
		case "(":
			// Exclude invalid sequences, which cannot appear in
			// valid miniscripts: "((", ")(", ",(".
			if i > 0 && (tokens[i-1] == "(" || tokens[i-1] == ")" ||
				tokens[i-1] == ",") {

				return nil, fmt.Errorf("the sequence %s%s is "+
					"invalid", tokens[i-1], token)
			}

		case ",", ")":
			// End of a function argument - take the argument and
			// add it to the parent's argument list. If there is no
			// parent, the expression is unbalanced, e.g. `f(X))`.
			//
			// Exclude invalid sequences, which cannot appear in
			// valid miniscripts: "(,", "()", ",,", ",)".
			if i > 0 && (tokens[i-1] == "(" || tokens[i-1] == ",") {
				return nil, fmt.Errorf("the sequence %s%s is "+
					"invalid", tokens[i-1], token)
			}

			arg := stack.pop()
			parent := stack.top()
			if arg == nil || parent == nil {
				return nil, errors.New("unbalanced")
			}
			parent.args = append(parent.args, arg)

		default:
			if i > 0 && tokens[i-1] == ")" {
				return nil, fmt.Errorf("the sequence %s%s is "+
					"invalid", tokens[i-1], token)
			}

			// Split wrappers from identifier if they exist, e.g. in
			// "dv:older", "dv" are wrappers and "older" is the
			// identifier. We use strings.Cut instead of
			// strings.Split to avoid allocating a slice for every
			// token, the vast majority of which have no colon.
			var wrappers, identifier string
			before, after, found := strings.Cut(token, ":")
			if !found {
				// No colon => Only an identifier.
				identifier = before
			} else {
				wrappers, identifier = before, after

				// A second colon is not allowed; Cut only split
				// on the first one, so any leftover colon in
				// the identifier is an error.
				if strings.ContainsRune(identifier, ':') {
					return nil, fmt.Errorf("invalid "+
						"number of colons in token: %s",
						token)
				}
				if wrappers == "" {
					return nil, fmt.Errorf("no wrappers "+
						"found before colon before "+
						"identifier: %s", identifier)
				}
				if identifier == "" {
					return nil, fmt.Errorf("no identifier "+
						"found after colon after "+
						"wrappers: %s", wrappers)
				}
			}

			stack.push(&AST{
				ctx:        ctx,
				wrappers:   wrappers,
				identifier: identifier,
			})
		}
	}

	// Every argument must have been attached, leaving exactly one root.
	if stack.size() != 1 {
		return nil, errors.New("unbalanced")
	}

	return stack.top(), nil
}

// argCheck checks that each identifier is a known miniscript identifier and
// that it has the correct number of arguments, e.g. `andor(X,Y,Z)` must have
// three arguments, etc.
func argCheck(node *AST) (*AST, error) {
	// Helper function to check that this node has a specific number of
	// arguments.
	expectArgs := func(num int) error {
		if len(node.args) != num {
			return fmt.Errorf("%s expects %d arguments, got %d",
				node.identifier, num, len(node.args))
		}
		return nil
	}

	// Helper function to check that an argument is a value (a key, a hash
	// or a number) and not a sub expression. Wrappers are only defined for
	// sub expressions, and the tree passes (expandWrappers among them) do
	// not descend into value arguments, so a wrapper on one would be
	// silently dropped from the compiled script while remaining visible in
	// the tree. rust-miniscript rejects them as well.
	checkValueArg := func(arg *AST) error {
		if len(arg.args) > 0 {
			return fmt.Errorf("argument of %s must not contain "+
				"subexpressions", node.identifier)
		}
		if arg.wrappers != "" {
			return fmt.Errorf("argument %q of %s must not have "+
				"wrappers, got %q", arg.identifier,
				node.identifier, arg.wrappers)
		}
		return nil
	}
	switch node.identifier {
	case f_0, f_1:
		if err := expectArgs(0); err != nil {
			return nil, err
		}

	case f_pk_k, f_pk_h, f_pk, f_pkh, f_sha256, f_ripemd160, f_hash256,
		f_hash160:

		if err := expectArgs(1); err != nil {
			return nil, err
		}
		if err := checkValueArg(node.args[0]); err != nil {
			return nil, err
		}

	case f_older, f_after:
		if err := expectArgs(1); err != nil {
			return nil, err
		}
		lockArg := node.args[0]
		if err := checkValueArg(lockArg); err != nil {
			return nil, err
		}

		// Timelocks accept the positive, signed 31-bit range. Store the
		// parsed value for later type, Script and satisfaction passes.
		n, err := strconv.ParseUint(lockArg.identifier, 10, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"%s(k) => k must be an unsigned integer, but "+
					"got: %s", node.identifier,
				lockArg.identifier)
		}
		lockArg.num = n
		if n < 1 || n >= (1<<31) {
			return nil, fmt.Errorf("%s(n) -> n must 1 ≤ n < 2^31, "+
				"but got: %s", node.identifier, lockArg.identifier)
		}

	case f_andor:
		if err := expectArgs(3); err != nil {
			return nil, err
		}

	case f_and_v, f_and_b, f_and_n, f_or_b, f_or_c, f_or_d, f_or_i:
		if err := expectArgs(2); err != nil {
			return nil, err
		}

	case f_thresh, f_multi, f_multi_a, f_sortedmulti_a:
		if len(node.args) < 2 {
			return nil, fmt.Errorf("%s must have at least two "+
				"arguments", node.identifier)
		}
		thresholdArg := node.args[0]
		if err := checkValueArg(thresholdArg); err != nil {
			return nil, err
		}

		// The leading number is a threshold, not a child expression; it
		// must select at least one and no more than all following args.
		k, err := strconv.ParseUint(thresholdArg.identifier, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s(k, ...) => k must be an "+
				"integer, but got: %s", node.identifier,
				thresholdArg.identifier)
		}
		thresholdArg.num = k
		numSubs := len(node.args) - 1
		if k < 1 || k > uint64(numSubs) {
			return nil, fmt.Errorf("%s(k) -> k must 1 ≤ k ≤ n, "+
				"but got: %s", node.identifier, thresholdArg.identifier)
		}
		if node.identifier != f_thresh {
			isMultiA := node.identifier == f_multi_a ||
				node.identifier == f_sortedmulti_a

			// multi (OP_CHECKMULTISIG) is only valid in P2WSH,
			// multi_a and sortedmulti_a (OP_CHECKSIGADD) only in
			// P2TR.
			if node.identifier == f_multi && node.ctx == P2TR {
				return nil, fmt.Errorf("multi is not allowed " +
					"in the P2TR context, use multi_a")
			}
			if isMultiA && node.ctx != P2TR {
				return nil, fmt.Errorf("%s is not allowed in "+
					"the %v context, use multi",
					node.identifier, node.ctx)
			}

			// multi allows up to 20 keys (OP_CHECKMULTISIG),
			// multi_a up to 999 (OP_CHECKSIGADD).
			maxKeys := multisigMaxKeys
			if isMultiA {
				maxKeys = checkSigAddMaxKeys
			}
			if numSubs > maxKeys {
				return nil, fmt.Errorf("number of %s keys "+
					"cannot exceed %d", node.identifier,
					maxKeys)
			}

			// Multisig keys are variables, they can't have
			// subexpressions or wrappers.
			for _, arg := range node.args {
				if err := checkValueArg(arg); err != nil {
					return nil, err
				}
			}
		}

	default:
		return nil, fmt.Errorf("unrecognized identifier: %s",
			node.identifier)
	}
	return node, nil
}

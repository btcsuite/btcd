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

// basicType records how a fragment composes with the surrounding stack:
// B leaves a Boolean, V verifies without leaving a result, K leaves a key for
// CHECKSIG, and W runs beneath one extra stack element.
type basicType string

const (
	typeB basicType = "B"
	typeV basicType = "V"
	typeK basicType = "K"
	typeW basicType = "W"
)

type properties struct {
	// Basic type properties.
	z, o, n, d, u bool

	// Malleability properties.
	// If `m`, a non-malleable satisfaction is guaranteed to exist.
	// The purpose of s/f/e is only to compute `m` and can be disregarded
	// afterward.
	m, s, f, e bool

	// canCollapseVerify enables checking if the rightmost script byte
	// produced by this node is OP_EQUAL, OP_CHECKSIG or OP_CHECKMULTISIG.
	//
	// If so, it can be converted into the VERIFY version if an ancestor is
	// the verify wrapper `v`, i.e. OP_EQUALVERIFY, OP_CHECKSIGVERIFY and
	// OP_CHECKMULTISIGVERIFY instead of using two opcodes, e.g.
	// `OP_EQUAL OP_VERIFY`.
	canCollapseVerify bool
}

// String returns property letters in a stable diagnostic order. The order
// has no type-system meaning; vector comparisons must treat them as a set.
func (p properties) String() string {
	// Keep the established order so diagnostic output remains comparable
	// across expressions and existing test vectors.
	s := strings.Builder{}
	if p.z {
		s.WriteRune('z')
	}
	if p.o {
		s.WriteRune('o')
	}
	if p.n {
		s.WriteRune('n')
	}
	if p.d {
		s.WriteRune('d')
	}
	if p.u {
		s.WriteRune('u')
	}
	if p.m {
		s.WriteRune('m')
	}
	if p.s {
		s.WriteRune('s')
	}
	if p.f {
		s.WriteRune('f')
	}
	if p.e {
		s.WriteRune('e')
	}
	return s.String()
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
//  2. expandWrappers: Unwraps the numbers before the colon, for example:
//     dv:older(144) is d(v(older(144)))
//  1. deSugar: Miniscript defines six instances of syntactic sugar. We replace
//     these with fixed equations.
//  1. typeCheck: Not all fragments compose with each other to produce a valid
//     Bitcoin Script and valid witness. This function checks that and sets the
//     types of the Miniscript fragments. Only if the top level basic type is of
//     type B the miniscript is valid.
func ParseInsane(miniscript string, ctx Context) (*AST, error) {
	node, err := createAST(miniscript, ctx)
	if err != nil {
		return nil, err
	}

	// expandWrappers and deSugar create new nodes, so we stamp the context
	// onto every node of the (now final-shaped) tree right after them. The
	// preceding argCheck only ever inspects the original createAST nodes,
	// which already carry the context.
	setContext := func(node *AST) (*AST, error) {
		node.ctx = ctx
		return node, nil
	}

	transformers := []func(*AST) (*AST, error){
		argCheck,
		expandWrappers,
		deSugar,
		setContext,
		checkContextFragments,
		typeCheck,
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

	basicType  basicType
	props      properties
	wrappers   string
	identifier string

	// num is the parsed integer for when identifier is expected to be a
	// number, i.e. the first argument of older/after/multi/thresh. This is
	// not used otherwise.
	num uint64

	// sortedKeys is set on the multi_a a sortedmulti_a desugars to. Its
	// keys are sorted by their serialization once they are known, which is
	// what makes the two fragments differ (BIP387).
	sortedKeys bool

	args []*AST
}

// formattedType returns the basic type (B, V, K or W) followed by all type
// properties.
func (a *AST) formattedType() string {
	return fmt.Sprintf("%s%s", a.basicType, a.props)
}

// IsValidTopLevel checks whether this node is valid as a script on its own.
func (a *AST) IsValidTopLevel() error {
	// Top-level expression must be of type "B".
	return a.expectBasicType(typeB)
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

// expectBasicType is a helper function to check that this node has a specific
// type.
func (a *AST) expectBasicType(typ basicType) error {
	if a.basicType != typ {
		return fmt.Errorf("expression `%s` expected to have type %s, "+
			"but is type %s", a.identifier, typ, a.basicType)
	}
	return nil
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

// checkContextFragments rejects fragments that must not be used in the script
// context the expression is parsed in.
//
// It runs after deSugar, so it sees the fragments the syntactic sugar expands
// to: the u: and l: wrappers are or_i, and the t: wrapper is and_v.
func checkContextFragments(node *AST) (*AST, error) {
	if node.ctx != Legacy {
		return node, nil
	}

	// Both or_i and d: select a branch with an OP_IF, whose argument is
	// only required to be minimally encoded from segwit on
	// (SCRIPT_VERIFY_MINIMALIF is not a consensus rule for pre-segwit
	// scripts). A third party can therefore replace the branch selector of
	// a legacy satisfaction with any other non-zero value, changing the
	// transaction id without invalidating the spend, which is why
	// rust-miniscript rejects both fragments in its Legacy context as well.
	switch node.identifier {
	case f_or_i:
		return nil, errors.New("or_i (and the u: and l: wrappers, " +
			"which are defined in terms of it) is malleable in " +
			"the Legacy context and not allowed there")

	case f_wrap_d:
		return nil, errors.New("the d: wrapper is malleable in the " +
			"Legacy context and not allowed there")
	}

	return node, nil
}

// expandWrappers applies wrappers (the characters before a colon), e.g.
// `ascd:X` => `a(s(c(d(X))))`.
func expandWrappers(node *AST) (*AST, error) {
	const allWrappers = "asctdvjnlu"

	wrappers := []rune(node.wrappers)
	node.wrappers = ""
	for i := len(wrappers) - 1; i >= 0; i-- {
		wrapper := wrappers[i]
		if !strings.ContainsRune(allWrappers, wrapper) {
			return nil, fmt.Errorf("unknown wrapper: %s",
				string(wrapper))
		}
		node = &AST{identifier: string(wrapper), args: []*AST{node}}
	}
	return node, nil
}

// deSugar replaces syntactic sugar with the final form.
func deSugar(node *AST) (*AST, error) {
	switch node.identifier {
	case f_pk: // pk(key) = c:pk_k(key)
		return &AST{
			identifier: f_wrap_c,
			args: []*AST{
				{
					identifier: f_pk_k,
					args:       node.args,
				},
			},
		}, nil

	case f_pkh: // pkh(key) = c:pk_h(key)
		return &AST{
			identifier: f_wrap_c,
			args: []*AST{
				{
					identifier: f_pk_h,
					args:       node.args,
				},
			},
		}, nil

	case f_and_n: // and_n(X,Y) = andor(X,Y,0)
		return &AST{
			identifier: f_andor,
			args: []*AST{
				node.args[0],
				node.args[1],
				{identifier: f_0},
			},
		}, nil

	case f_wrap_t: // t:X = and_v(X,1)
		return &AST{
			identifier: f_and_v,
			args: []*AST{
				node.args[0],
				{identifier: f_1},
			},
		}, nil

	case f_wrap_l: // l:X = or_i(0,X)
		return &AST{
			identifier: f_or_i,
			args: []*AST{
				{identifier: f_0},
				node.args[0],
			},
		}, nil

	case f_sortedmulti_a: // sortedmulti_a(k,...) = sorted multi_a(k,...)
		return &AST{
			identifier: f_multi_a,
			args:       node.args,
			sortedKeys: true,
		}, nil

	case f_wrap_u: // u:X = or_i(X,0)
		return &AST{
			identifier: f_or_i,
			args: []*AST{
				node.args[0],
				{identifier: f_0},
			},
		}, nil
	}

	return node, nil
}

// typeCheck validates the BIP379 composition rules and derives the parent's
// basic type and z/o/n/d/u properties from already-checked children. The
// argument names x, y and z follow the fragment notation in the BIP's table.
func typeCheck(node *AST) (*AST, error) {
	// Each case checks the child contract before deriving the parent. Keep
	// these formulas aligned with the specification rather than combining
	// cases whose current property assignments merely happen to coincide.
	switch node.identifier {
	case f_0:
		node.basicType = typeB
		node.props.z = true
		node.props.u = true
		node.props.d = true

	case f_1:
		node.basicType = typeB
		node.props.z = true
		node.props.u = true

	case f_pk_k:
		node.basicType = typeK
		node.props.o = true
		node.props.n = true
		node.props.d = true
		node.props.u = true

	case f_pk_h:
		node.basicType = typeK
		node.props.n = true
		node.props.d = true
		node.props.u = true

	case f_older, f_after:
		node.basicType = typeB
		node.props.z = true

	case f_sha256, f_ripemd160, f_hash256, f_hash160:
		node.basicType = typeB
		node.props.o = true
		node.props.n = true
		node.props.d = true
		node.props.u = true

	case f_andor:
		x, y, z := node.args[0], node.args[1], node.args[2]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.d || !x.props.u {
			return nil, fmt.Errorf("wrong properties on `%s` in "+
				"the first argument of `%s`", x.identifier,
				node.identifier)
		}
		if y.basicType != typeB && y.basicType != typeK &&
			y.basicType != typeV {

			return nil, fmt.Errorf("in `%s`, the second argument "+
				"type is not B, K or V, but: %s",
				node.identifier, y.basicType)
		}
		if z.basicType != y.basicType {
			return nil, fmt.Errorf("in `%s`, the third of the "+
				"argument is not the same as the type of the "+
				"second argument, which is: %s",
				node.identifier, y.basicType)
		}
		node.basicType = y.basicType
		node.props.z = x.props.z && y.props.z && z.props.z
		node.props.o = (x.props.z && y.props.o && z.props.o) ||
			(x.props.o && y.props.z && z.props.z)
		node.props.u = y.props.u && z.props.u
		node.props.d = z.props.d

	case f_and_v:
		x, y := node.args[0], node.args[1]
		if err := x.expectBasicType(typeV); err != nil {
			return nil, err
		}
		if y.basicType != typeB && y.basicType != typeK &&
			y.basicType != typeV {

			return nil, fmt.Errorf("in `%s`, the second argument "+
				"type is not B, K or V, but: %s",
				node.identifier, y.basicType)
		}
		node.basicType = y.basicType
		node.props.z = x.props.z && y.props.z
		node.props.o = (x.props.z && y.props.o) ||
			(y.props.z && x.props.o)
		node.props.n = x.props.n || (x.props.z && y.props.n)
		node.props.u = y.props.u

	case f_and_b:
		x, y := node.args[0], node.args[1]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if err := y.expectBasicType(typeW); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = x.props.z && y.props.z
		node.props.o = (x.props.z && y.props.o) ||
			(y.props.z && x.props.o)
		node.props.n = x.props.n || (x.props.z && y.props.n)
		node.props.d = x.props.d && y.props.d
		node.props.u = true

	case f_or_b:
		x, z := node.args[0], node.args[1]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.d {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", x.identifier,
				node.identifier)
		}
		if err := z.expectBasicType(typeW); err != nil {
			return nil, err
		}
		if !z.props.d {
			return nil, fmt.Errorf(
				"wrong properties on `%s`, the second "+
					"argument of `%s`", z.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.z = x.props.z && z.props.z
		node.props.o = (x.props.z && z.props.o) ||
			(z.props.z && x.props.o)
		node.props.d = true
		node.props.u = true

	case f_or_c:
		x, z := node.args[0], node.args[1]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.d || !x.props.u {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", x.identifier,
				node.identifier)
		}
		if err := z.expectBasicType(typeV); err != nil {
			return nil, err
		}
		node.basicType = typeV
		node.props.z = x.props.z && z.props.z
		node.props.o = x.props.o && z.props.z

	case f_or_d:
		x, z := node.args[0], node.args[1]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.d || !x.props.u {
			return nil, fmt.Errorf(
				"wrong properties on `%s`, the first argument "+
					"of `%s`", x.identifier,
				node.identifier)
		}
		if err := z.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = x.props.z && z.props.z
		node.props.o = x.props.o && z.props.z
		node.props.d = z.props.d
		node.props.u = z.props.u

	case f_or_i:
		x, z := node.args[0], node.args[1]
		if x.basicType != typeB && x.basicType != typeK &&
			x.basicType != typeV {

			return nil, errors.New("or_i: wrong type of first " +
				"argument")
		}
		if z.basicType != x.basicType {
			return nil, errors.New("or_i: wrong type of second " +
				"argument")
		}
		node.basicType = x.basicType
		node.props.o = x.props.z && z.props.z
		node.props.u = x.props.u && z.props.u
		node.props.d = x.props.d || z.props.d

	case f_thresh:
		// The first child starts the sum. Later children must preserve
		// that accumulator, so X1 is Bdu and the remaining children
		// Wdu.
		if err := node.args[1].expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !node.args[1].props.d || !node.args[1].props.u {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"second argument of `%s`",
				node.args[1].identifier, node.identifier)
		}
		for i := 2; i < len(node.args); i++ {
			arg := node.args[i]
			if err := arg.expectBasicType(typeW); err != nil {
				return nil, err
			}
			if !arg.props.d || !arg.props.u {
				return nil, fmt.Errorf("wrong properties on "+
					"`%s`, argument #%d of `%s`",
					arg.identifier, i+1, node.identifier)
			}
		}

		node.basicType = typeB

		// z: all sub expressions read zero elements from the stack.
		// o: the sub expressions read exactly one element in total,
		// i.e. exactly one reads one and all others read zero. This
		// mirrors rust-miniscript's num_args computation: a z sub
		// contributes 0, an o sub contributes 1, anything else
		// contributes 2, and o holds iff the total is exactly 1
		// (reachable only for a single-sub thresh, or in general a
		// thresh whose subs are all z except one o).
		node.props.z = true
		numArgs := 0
		for _, arg := range node.args[1:] {
			node.props.z = node.props.z && arg.props.z
			switch {
			case arg.props.z:

			case arg.props.o:
				numArgs++

			default:
				numArgs += 2
			}
		}
		node.props.o = numArgs == 1
		node.props.d = true
		node.props.u = true

	case f_multi:
		node.basicType = typeB
		node.props.n = true
		node.props.d = true
		node.props.u = true

	case f_multi_a:
		// Unlike multi, multi_a is not `n`: it can be dissatisfied with
		// an all-zero witness (input Any, not AnyNonZero).
		node.basicType = typeB
		node.props.d = true
		node.props.u = true

	case f_wrap_a:
		x := node.args[0]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeW
		node.props.d = x.props.d
		node.props.u = x.props.u

	case f_wrap_s:
		x := node.args[0]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.o {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", x.identifier,
				node.identifier)
		}
		node.basicType = typeW
		node.props.d = x.props.d
		node.props.u = x.props.u

	case f_wrap_c:
		x := node.args[0]
		if err := x.expectBasicType(typeK); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.o = x.props.o
		node.props.n = x.props.n
		node.props.d = x.props.d
		node.props.u = true

	case f_wrap_d:
		x := node.args[0]
		if err := x.expectBasicType(typeV); err != nil {
			return nil, err
		}
		if !x.props.z {
			return nil, fmt.Errorf("wrong property of `%s`, the "+
				"first argument of `%s`", x.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.o = true
		node.props.n = true
		node.props.d = true

		// The OP_IF of a d: consumes the topmost witness element, which
		// in Tapscript can only be the single byte 0x01, since
		// MINIMALIF is a consensus rule there. That makes the fragment
		// unit in that context, but not under P2WSH consensus, which
		// permits other Script-true values (BIP379).
		node.props.u = node.ctx == P2TR

	case f_wrap_v:
		x := node.args[0]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeV
		node.props.z = x.props.z
		node.props.o = x.props.o
		node.props.n = x.props.n

	case f_wrap_j:
		x := node.args[0]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !x.props.n {
			return nil, fmt.Errorf("wrong property of `%s`, the "+
				"first argument of `%s`", x.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.o = x.props.o
		node.props.n = true
		node.props.d = true
		node.props.u = x.props.u

	case f_wrap_n:
		x := node.args[0]
		if err := x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = x.props.z
		node.props.o = x.props.o
		node.props.n = x.props.n
		node.props.d = x.props.d
		node.props.u = true

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}
	return node, nil
}

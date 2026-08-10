package miniscript

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/btcsuite/btcd/address/v2"
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

	// maxOpsPerScript is the maximum number of non-push operations per
	// script. This is a consensus rule in the P2WSH context; it does not
	// apply in the Tapscript context.
	maxOpsPerScript = 201

	// maxStandardP2WSHStackItems is the maximum number of witness stack
	// items a standard P2WSH spend may have, not counting the witness
	// script that is pushed as the final witness element. This is a
	// standardness rule.
	maxStandardP2WSHStackItems = 100

	// maxRedeemScriptSize is the maximum size in bytes of the redeem script
	// of a P2SH output. The redeem script is pushed as a single data
	// element in the spending scriptSig, so the consensus limit on the size
	// of a script element applies to it: an output whose redeem script is
	// larger can never be spent.
	maxRedeemScriptSize = 520

	// maxScriptSigSize is the maximum size in bytes of a standard
	// scriptSig. This is a standardness rule, so a spend needing a larger
	// scriptSig is not relayed.
	maxScriptSigSize = 1650

	// maxStackSize is the maximum number of stack elements that may exist
	// at any point before or during script execution (a consensus rule). It
	// bounds the sum of the initial witness elements and the elements
	// pushed during execution, and applies to Tapscript as well as P2WSH.
	maxStackSize = 1000

	// multisigMaxKeys is the maximum number of keys in a P2WSH multisig
	// (OP_CHECKMULTISIG).
	multisigMaxKeys = 20

	// checkSigAddMaxKeys is the maximum number of keys in a Tapscript
	// multi_a (OP_CHECKSIGADD) expression.
	checkSigAddMaxKeys = 999

	// maxNestingDepth is the maximum nesting depth of a miniscript
	// expression, where every sub expression and every wrapper counts as
	// one level.
	//
	// The limit exists because every tree pass (the analysis passes run by
	// Parse, but also Script, Satisfy, Clone, Keys, Lift and DrawTree)
	// recurses once per level, and Go grows a goroutine stack only up to a
	// hard limit, after which the runtime throws a fatal stack overflow
	// that recover() cannot catch, killing the process rather than the
	// request. Roughly one megabyte of `n:` wrappers was enough to reach
	// it.
	//
	// The value matches rust-miniscript's MAX_RECURSION_DEPTH, which exists
	// for the same reason and is far beyond any legitimate expression. See
	// https://github.com/sipa/miniscript/pull/5 for a discussion of the
	// number.
	maxNestingDepth = 402
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

// keyPushLen returns the length of a public key data push in the script, which
// is the key length plus one byte for the length prefix.
func (c Context) keyPushLen() int {
	return c.keyLen() + 1
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
	// If `m`, the fragment meets the structural non-malleability rules;
	// this does not imply a satisfaction is available for given assets.
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

// Parse a miniscript expression to be executed in the given script context
// (P2WSH or P2TR). The context determines the allowed fragments, the public key
// encoding and the resource limits.
//
// The parsed expression is checked to be sane, i.e. safe to use as a script on
// its own: it must be a valid base expression (type "B"), non-malleable, must
// require a signature, must not mix height- and time-based time locks on a
// single spending path, and its script and satisfaction must stay within the
// resource limits of the context. This mirrors rust-miniscript's `from_str`,
// which validates with `Ctx::SANE`.
//
// Use ParseInsane to parse an expression without these checks, for example to
// analyze a script that is known not to be sane.
func Parse(miniscript string, ctx Context) (*AST, error) {
	node, err := ParseInsane(miniscript, ctx)
	if err != nil {
		return nil, err
	}

	if err := node.IsSane(); err != nil {
		return nil, fmt.Errorf("miniscript is not sane: %w", err)
	}

	return node, nil
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
//  1. canCollapseVerify: If the rightmost script byte of a node is OP_EQUAL,
//     OP_CHECKSIG or OP_CHECKMULTISIG. We can convert it to the VERIFY version
//     of the opcode, e.g. OP_EQUALVERIFY.
//  2. malleabilityCheck: Checks each node if it is malleable (checking that the
//     transaction hash can not be changes without altering the content).
//  1. computeScriptLen: Simply computes the script length.
//  2. computeOpCount: Counts the amount of opcodes the script contains.
//  3. computeStackSize: Computes the maximum witness stack size needed to
//     (dis)satisfy the script.
//  1. computeTimelocks: Computes the time lock info used to detect time lock
//     mixing.
func ParseInsane(miniscript string, ctx Context) (*AST, error) {
	node, err := createAST(miniscript, ctx)
	if err != nil {
		return nil, err
	}

	// Reject expressions that nest too deeply before anything walks the
	// tree recursively, since that is what a deeply nested expression would
	// otherwise crash.
	if err := checkNestingDepth(node); err != nil {
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
		canCollapseVerify,
		malleabilityCheck,
		computeScriptLen,
		computeOpCount,
		computeStackSize,
		computeSatSize,
		computeExecStack,
		computeTimelocks,
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
	// ctx is the script context (P2WSH, P2TR or Legacy) the expression is
	// parsed in. It is the same for every node of a tree.
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

	// For key arguments, value holds a compressed or x-only public key,
	// according to the script context.
	// For hash arguments, this will be the 32 bytes (sha256, hash256) or
	// 20 bytes (ripemd160, hash160) hash.
	value     []byte
	args      []*AST
	scriptLen int
	opCount   ops

	// stackSize is the maximum number of witness stack elements needed to
	// satisfy or dissatisfy this node.
	stackSize stackSize

	// satSize is the maximum witness byte size (and element count) needed
	// to satisfy or dissatisfy this node, used for weight estimation.
	satSize witSize

	// execStack is the maximum number of stack elements pushed during
	// execution (beyond the initial witness) to satisfy or dissatisfy this
	// node. Both segwit contexts use it to enforce the stack element limit.
	execStack execSize

	// timelock tracks the height- and time-based time locks that may be
	// encountered when satisfying this node, used to detect time lock
	// mixing.
	timelock timelockInfo
}

// formattedType returns the basic type (B, V, K or W) followed by all type
// properties.
func (a *AST) formattedType() string {
	return fmt.Sprintf("%s%s", a.basicType, a.props)
}

// isValid checks the encoded script size, independently of whether this node
// is a valid top-level expression or has a usable satisfaction.
func (a *AST) isValid() error {
	if a.scriptLen > a.ctx.maxScriptSize() {
		return fmt.Errorf("the script size is %v, which is larger "+
			"than the maximum script size of %v in the %v context",
			a.scriptLen, a.ctx.maxScriptSize(), a.ctx)
	}
	return nil
}

// IsValidTopLevel checks whether this node is valid as a script on its own.
func (a *AST) IsValidTopLevel() error {
	if err := a.isValid(); err != nil {
		return err
	}

	// Top-level expression must be of type "B".
	return a.expectBasicType(typeB)
}

// validSatisfactions checks whether successful non-malleable satisfactions are
// guaranteed to be valid and that a satisfaction does not violate the context's
// resource limits.
func (a *AST) validSatisfactions() error {
	if err := a.isValid(); err != nil {
		return err
	}

	// Consensus rule in both segwit contexts: the number of stack elements
	// at any point before or during execution is limited to 1000, i.e. the
	// initial witness elements plus the elements pushed while executing the
	// script. It cannot be reached within the 520-byte script limit of the
	// legacy context, which is why that context does not check it, matching
	// rust-miniscript.
	if a.ctx != Legacy && a.stackSize.sat.valid && a.execStack.sat.valid {
		stackElems := a.maxWitnessSize() + a.maxExecStackSize()
		if stackElems > maxStackSize {
			return fmt.Errorf("the satisfaction requires a stack of "+
				"%d elements, which is larger than the "+
				"consensus limit of %d", stackElems,
				maxStackSize)
		}
	}

	switch a.ctx {
	case Legacy:
		// Consensus rule, in the legacy context as well: the number of
		// non-push operations is limited to 201.
		if a.maxOpCount() > maxOpsPerScript {
			return fmt.Errorf("the script requires a maximum "+
				"number of %d ops, which is larger than the "+
				"consensus limit of %d", a.maxOpCount(),
				maxOpsPerScript)
		}

		// Standardness rule: a legacy satisfaction lives in the
		// scriptSig, together with the redeem script it spends, and a
		// scriptSig larger than 1650 bytes is not relayed. The stack
		// element limit of 1000 cannot be reached within the 520-byte
		// redeem script limit, so it is not checked, matching
		// rust-miniscript.
		if a.satSize.sat.valid {
			scriptSig := a.satSize.sat.size +
				pushScriptSize(a.scriptLen)
			if scriptSig > maxScriptSigSize {
				return fmt.Errorf("the satisfaction requires a "+
					"scriptSig of %d bytes, which is larger "+
					"than the standardness limit of %d",
					scriptSig, maxScriptSigSize)
			}
		}

	case P2WSH:
		// P2WSH consensus rule: the number of non-push operations is
		// limited to 201.
		if a.maxOpCount() > maxOpsPerScript {
			return fmt.Errorf("the script requires a maximum "+
				"number of %d ops, which is larger than the "+
				"consensus limit of %d", a.maxOpCount(),
				maxOpsPerScript)
		}

		// P2WSH standardness rule: the number of witness stack elements
		// a spend may push is limited. The witness script itself, which
		// is pushed as the final witness stack element, is excluded
		// from the count, as BIP379 states and Core implements
		// (policy.cpp:312 in Core c4fbd3c7211).
		if a.stackSize.sat.valid {
			witnessItems := a.maxWitnessSize()
			if witnessItems > maxStandardP2WSHStackItems {
				return fmt.Errorf("the satisfaction requires "+
					"%d witness stack elements, which is "+
					"larger than the standardness limit "+
					"of %d", witnessItems,
					maxStandardP2WSHStackItems)
			}
		}

	case P2TR:
		// Tapscript has no op count limit and no standardness limit on
		// the number of witness elements, so the stack element limit
		// checked above is all that applies to it.
	}

	return nil
}

// pushScriptSize returns the number of bytes it takes to push a script of the
// given size as a single data element, i.e. the push opcode (with its length
// bytes) plus the script itself. In the legacy context this is what the redeem
// script contributes to the scriptSig.
func pushScriptSize(scriptLen int) int {
	switch {
	case scriptLen < 76:
		return 1 + scriptLen

	case scriptLen < 256:
		return 2 + scriptLen

	default:
		return 3 + scriptLen
	}
}

// isSaneSubexpression checks whether the apparent policy of this node matches
// its script semantics. Doesn't guarantee it is a safe script on its own.
func (a *AST) isSaneSubexpression() error {
	if err := a.validSatisfactions(); err != nil {
		return err
	}
	if !a.props.m {
		return errors.New("malleable")
	}

	// A script that mixes height-based and time-based time locks of the
	// same kind (absolute or relative) on a single spending path has a
	// branch that can never be satisfied, see
	// https://medium.com/blockstream/dont-mix-your-timelocks-d9939b665094.
	if a.timelock.containsCombination {
		return errors.New(
			"contains a combination of height-based and time-" +
				"based time locks on a single spending path",
		)
	}

	return a.checkDuplicateKeys()
}

// checkDuplicateKeys checks that no public key appears more than once in the
// expression. BIP379's security analysis assumes they are all distinct, and a
// repeated key makes the analysis wrong: a signature made for one occurrence
// can be replayed into the other, so a policy that reads as needing two
// signatures may be satisfiable with one. Core rejects the same thing as part
// of its sanity check (miniscript.h:1699 in Core c4fbd3c7211).
//
// The keys are compared as the identifiers they are written as, which is all
// there is to compare before ApplyVars substitutes the bytes. Two different
// identifiers that resolve to the same key are caught by the check ApplyVars
// runs on the substituted keys.
func (a *AST) checkDuplicateKeys() error {
	keys := a.Keys()
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate key %s", key)
		}
		seen[key] = struct{}{}
	}

	return nil
}

// IsSane checks whether this node is safe as a script on its own.
func (a *AST) IsSane() error {
	if err := a.IsValidTopLevel(); err != nil {
		return err
	}
	if err := a.isSaneSubexpression(); err != nil {
		return err
	}
	if !a.props.s {
		return errors.New("does not need signature")
	}
	return nil
}

// drawTree renders this node and its descendants for diagnostics. Writer errors
// are ignored because DrawTree supplies an in-memory strings.Builder.
func (a *AST) drawTree(w io.Writer, indent string) {
	// Show the expression's analyzed type next to its symbolic identifier;
	// include substituted bytes only when they add information.
	if a.wrappers != "" {
		_, _ = fmt.Fprintf(w, "%s:", a.wrappers)
	}
	_, _ = fmt.Fprint(w, a.identifier)
	typ := a.formattedType()
	if a.props.canCollapseVerify {
		typ += "v"
	}
	if typ != "" {
		_, _ = fmt.Fprintf(w, " [%s]", typ)
	}
	if a.value != nil {
		h := hex.EncodeToString(a.value)
		if h != a.identifier {
			_, _ = fmt.Fprintf(w, " [%x]", a.value)
		}
	}
	_, _ = fmt.Fprintln(w)

	// Keep sibling connectors aligned by rune count rather than byte
	// length, since the branch markers themselves are multibyte characters.
	for i, arg := range a.args {
		mark := ""
		delim := ""
		if i == len(a.args)-1 {
			mark = "└──"
		} else {
			mark = "├──"
			delim = "|"
		}
		_, _ = fmt.Fprintf(w, "%s%s", indent, mark)
		padLen := utf8.RuneCountInString(arg.identifier) +
			utf8.RuneCountInString(mark) -
			1 - len(delim)
		padding := strings.Repeat(" ", padLen)
		arg.drawTree(w, indent+delim+padding)
	}
}

// DrawTree returns a diagnostic tree with fragment types and substituted
// values. It does not modify the parsed expression.
func (a *AST) DrawTree() string {
	var b strings.Builder
	a.drawTree(&b, "")
	return b.String()
}

// Keys returns the public key identifiers appearing in the expression, in the
// order they appear. These are the arguments of the pk_k, pk_h, multi and
// multi_a fragments. Hash values and time lock numbers are not keys and are not
// returned.
func (a *AST) Keys() []string {
	var keys []string
	a.collectKeys(&keys)
	return keys
}

// collectKeys appends the key identifiers of this node and its sub expressions
// to out, in order.
func (a *AST) collectKeys(out *[]string) {
	switch a.identifier {
	case f_pk_k, f_pk_h, f_pk, f_pkh:
		*out = append(*out, a.args[0].identifier)

	case f_multi, f_multi_a:
		for _, arg := range a.args[1:] {
			*out = append(*out, arg.identifier)
		}

	case f_0, f_1, f_older, f_after, f_sha256, f_hash256, f_ripemd160,
		f_hash160:

		// These fragments contain no public keys.

	case f_thresh:
		// The first argument is the threshold number, the rest are sub
		// expressions.
		for _, arg := range a.args[1:] {
			arg.collectKeys(out)
		}

	default:
		for _, arg := range a.args {
			arg.collectKeys(out)
		}
	}
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

// checkNestingDepth returns an error if the tree rooted at the given node nests
// deeper than maxNestingDepth. Every wrapper character counts as one level,
// because expandWrappers turns each of them into a node of its own.
//
// The tree is walked with an explicit stack: a recursive walk would itself
// overflow the goroutine stack on the very inputs this check exists to reject.
func checkNestingDepth(node *AST) error {
	type item struct {
		node *AST

		// depth is the nesting level of the node: its parent's level,
		// plus one for the node itself, plus one per wrapper it
		// carries.
		depth int
	}

	stack := []item{{node: node, depth: 1 + len(node.wrappers)}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if current.depth > maxNestingDepth {
			return fmt.Errorf("expression nests at least %d levels "+
				"deep, which is more than the maximum nesting "+
				"depth of %d", current.depth, maxNestingDepth)
		}

		for i, arg := range current.node.args {
			if !current.node.isSubExpression(i) {
				continue
			}
			stack = append(stack, item{
				node:  arg,
				depth: current.depth + 1 + len(arg.wrappers),
			})
		}
	}

	return nil
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

// Clone returns a deep copy of the AST. The copy shares no mutable state with
// the original, so it is safe to run ApplyVars (which assigns concrete key and
// hash values into the tree) on the clone without affecting the original.
//
// This lets a parsed expression be cached and reused: parsing runs the full
// analysis pipeline (tokenization, wrapper expansion, type/malleability checks,
// resource computation), all of which is independent of the concrete key
// values, whereas cloning only copies the resulting tree.
func (a *AST) Clone() *AST {
	if a == nil {
		return nil
	}

	// A shallow struct copy duplicates every value-typed field, including
	// the computed properties, script length, op count, stack/sat/exec
	// sizes and time lock info, which contain no pointers, slices or maps.
	clone := *a

	// The value bytes and argument list are the only reference-typed
	// fields, so they need to be copied explicitly to fully decouple the
	// clone from the original.
	if a.value != nil {
		clone.value = append([]byte(nil), a.value...)
	}
	if a.args != nil {
		clone.args = make([]*AST, len(a.args))
		for i, arg := range a.args {
			clone.args[i] = arg.Clone()
		}
	}

	return &clone
}

// ApplyVars replaces key and hash values in the miniscript. It must be called
// before running Script() or Satisfy().
//
// The callback should return `nil, nil` if the variable is unknown. In this
// case, the identifier itself will be parsed as the value (hex-encoded pubkey,
// hex-encoded hash value).
//
// ApplyVars mutates the tree and retains the callback's returned slices. The
// caller must not mutate those slices while the tree is in use. Clone first
// when reusing a parsed template; an error may leave partial substitutions.
func (a *AST) ApplyVars(
	lookupVar func(identifier string) ([]byte, error)) error {

	// Different symbolic identifiers can resolve to the same concrete key.
	// Reject aliases across the entire tree, not just within one multisig.
	allPubKeys := map[string]struct{}{}

	_, err := a.apply(func(node *AST) (*AST, error) {
		switch node.identifier {
		case f_pk_k, f_pk_h, f_multi, f_multi_a:
			var keyArgs []*AST
			if node.identifier == f_multi ||
				node.identifier == f_multi_a {

				keyArgs = node.args[1:]
			} else {
				keyArgs = node.args[:1]
			}
			for _, arg := range keyArgs {
				key, err := lookupVar(arg.identifier)
				if err != nil {
					return nil, err
				}
				if key == nil {
					// If the key was not a variable, assume
					// it's the key value directly encoded
					// as hex.
					key, err = hex.DecodeString(
						arg.identifier,
					)
					if err != nil {
						return nil, err
					}
				}
				if len(key) != node.ctx.keyLen() {
					return nil, fmt.Errorf("pubkey "+
						"argument of %s expected to "+
						"be of size %d, but got %d",
						node.identifier,
						node.ctx.keyLen(), len(key))
				}

				pubKeyHex := hex.EncodeToString(key)
				if _, ok := allPubKeys[pubKeyHex]; ok {
					return nil, fmt.Errorf("duplicate key "+
						"found at %s (key=%s, arg "+
						"identifier=%s)",
						node.identifier, pubKeyHex,
						arg.identifier)
				}
				allPubKeys[pubKeyHex] = struct{}{}

				arg.value = key
			}

			// The keys of a sortedmulti_a are sorted by their
			// serialization before the script is built, which the
			// order of the satisfaction follows as well, since
			// every later pass works off the argument order.
			if node.sortedKeys {
				slices.SortFunc(keyArgs, func(a, b *AST) int {
					return bytes.Compare(a.value, b.value)
				})
			}

		case f_sha256, f_hash256, f_ripemd160, f_hash160:
			arg := node.args[0]
			hashLen := map[string]int{
				f_sha256:    32,
				f_hash256:   32,
				f_ripemd160: 20,
				f_hash160:   20,
			}[node.identifier]
			hashValue, err := lookupVar(arg.identifier)
			if err != nil {
				return nil, err
			}
			if hashValue == nil {
				// If the hash value was not a variable, assume
				// it's the hash value directly encoded as hex.
				hashValue, err = hex.DecodeString(
					node.args[0].identifier,
				)
				if err != nil {
					return nil, err
				}
			}
			if len(hashValue) != hashLen {
				return nil, fmt.Errorf("%s len must be %d, got"+
					"%d", node.identifier, hashLen,
					len(hashValue))
			}
			arg.value = hashValue

		}
		return node, nil
	})
	return err
}

// maxOpCount returns the maximum number of ops needed to satisfy this script
// in a non-malleable way.
func (a *AST) maxOpCount() int {
	return a.opCount.count + a.opCount.sat.value
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

// canCollapseVerify records whether the final emitted opcode has a VERIFY
// variant. Children have already been analyzed, allowing and_v and s: to
// inherit the property from the child that emits their final opcode.
func canCollapseVerify(node *AST) (*AST, error) {
	switch node.identifier {
	case f_sha256, f_ripemd160, f_hash256, f_hash160, f_thresh, f_multi,
		f_multi_a, f_wrap_c:

		// The final opcode of each of these is OP_EQUAL, OP_CHECKSIG,
		// OP_CHECKMULTISIG or OP_NUMEQUAL (multi_a), all of which have
		// a VERIFY variant.
		node.props.canCollapseVerify = true

	case f_and_v:
		otherProps := node.args[1].props
		node.props.canCollapseVerify = otherProps.canCollapseVerify

	case f_wrap_s:
		otherProps := node.args[0].props
		node.props.canCollapseVerify = otherProps.canCollapseVerify
	}

	return node, nil
}

// malleabilityCheck derives BIP379's m/s/f/e properties from already-analyzed
// children. The auxiliary s/f/e properties are meaningful only when m holds.
func malleabilityCheck(node *AST) (*AST, error) {
	// Keep the rules in fragment-table order so the Boolean conditions can
	// be checked directly against the specification. Availability of actual
	// signatures and preimages is handled by the satisfaction pass instead.
	switch node.identifier {
	case f_0:
		node.props.m = true
		node.props.s = true
		node.props.e = true

	case f_1:
		node.props.m = true
		node.props.f = true

	case f_pk_k, f_pk_h:
		node.props.m = true
		node.props.s = true
		node.props.e = true

	case f_older, f_after:
		node.props.m = true
		node.props.f = true

	case f_sha256, f_ripemd160, f_hash256, f_hash160:
		node.props.m = true

	case f_andor:
		x, y := node.args[0].props, node.args[1].props
		z := node.args[2].props
		node.props.m = x.m && y.m && z.m && (x.e && (x.s || y.s || z.s))
		node.props.s = z.s && (x.s || y.s)
		node.props.f = z.f && (x.s || y.f)
		node.props.e = z.e && (x.s || y.f)

	case f_and_v:
		x, y := node.args[0].props, node.args[1].props
		node.props.m = x.m && y.m
		node.props.s = x.s || y.s
		node.props.f = x.s || y.f

	case f_and_b:
		x, y := node.args[0].props, node.args[1].props
		node.props.m = x.m && y.m
		node.props.s = x.s || y.s
		node.props.f = x.f && y.f || x.s && x.f || y.s && y.f
		node.props.e = x.e && y.e && x.s && y.s

	case f_or_b:
		x, z := node.args[0].props, node.args[1].props
		node.props.m = x.m && z.m && (x.e && z.e && (x.s || z.s))
		node.props.s = x.s && z.s
		node.props.e = true

	case f_or_c:
		x, z := node.args[0].props, node.args[1].props
		node.props.m = x.m && z.m && (x.e && (x.s || z.s))
		node.props.s = x.s && z.s
		node.props.f = true

	case f_or_d:
		x, z := node.args[0].props, node.args[1].props
		node.props.m = x.m && z.m && (x.e && (x.s || z.s))
		node.props.s = x.s && z.s
		node.props.f = z.f

		// The specification uses e_z here. An earlier reference used
		// e_x && e_z, which differs only when m is false and e is
		// discarded below. A false m means non-malleable satisfaction
		// is not guaranteed, not that every satisfaction is malleable.
		// See https://github.com/sipa/miniscript/issues/128.
		node.props.e = z.e

	case f_or_i:
		x, z := node.args[0].props, node.args[1].props
		node.props.m = x.m && z.m && (x.s || z.s)
		node.props.s = x.s && z.s
		node.props.f = x.f && z.f
		node.props.e = x.e && z.f || z.e && x.f

	case f_thresh:
		k := node.args[0].num

		// A threshold must not leave more than k signature-free
		// children interchangeable. Requiring a signature tightens that
		// bound by one, ensuring at least one selected child must be
		// signed.
		notSCount := 0
		node.props.m = true
		for _, arg := range node.args[1:] {
			node.props.m = node.props.m && arg.props.m &&
				arg.props.e

			if !arg.props.s {
				notSCount++
			}
		}
		node.props.m = node.props.m && uint64(notSCount) <= k
		node.props.s = uint64(notSCount) <= k-1

		// BIP379's rule is "e=all are s". The m computed above already
		// requires every sub expression to be expressive, and a
		// threshold that does not meet that requirement carries no
		// malleability property at all (see the end of this function),
		// so the two readings of the rule agree.
		node.props.e = true
		for _, arg := range node.args[1:] {
			node.props.e = node.props.e && arg.props.s
		}

	case f_multi, f_multi_a:
		node.props.m = true
		node.props.s = true
		node.props.e = true

	case f_wrap_a, f_wrap_s:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = x.s
		node.props.f = x.f
		node.props.e = x.e

	case f_wrap_c:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = true
		node.props.f = x.f
		node.props.e = x.e

	case f_wrap_d:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = x.s
		node.props.e = true

	case f_wrap_v:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = x.s
		node.props.f = true

	case f_wrap_j:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = x.s
		node.props.e = x.f

	case f_wrap_n:
		x := node.args[0].props
		node.props.m = x.m
		node.props.s = x.s
		node.props.f = x.f
		node.props.e = x.e

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	// The s, f and e properties describe the satisfactions and
	// dissatisfactions of an expression only if it meets the malleability
	// requirement of every fragment it is built from, which is what m
	// tracks. Once an expression is malleable, so is every expression
	// containing it, and none of the three says anything about any of them,
	// so they are not carried for a malleable expression. BIP379 states
	// this below its malleability table.
	//
	// Clearing them here rather than at every use is sound because the m of
	// a parent requires the m of each of its sub expressions, so a parent
	// that consults a cleared property is malleable either way.
	if !node.props.m {
		node.props.s = false
		node.props.f = false
		node.props.e = false
	}

	return node, nil
}

// computeScriptLen derives the encoded script length from already-analyzed
// children. Concrete keys are unnecessary because the context fixes their size.
func computeScriptLen(node *AST) (*AST, error) {
	// Match the builder's minimal number encoding, including the
	// small-integer opcodes. A single int64 push cannot exceed the
	// builder's size limit.
	numPushLen := func(n int64) int {
		numPush, _ := txscript.NewScriptBuilder().AddInt64(n).Script()
		return len(numPush)
	}

	// Value arguments have no script of their own; expression children
	// supply the recursive contribution before fragment-specific opcodes
	// are added.
	argsSummed := 0
	for _, arg := range node.args {
		argsSummed += arg.scriptLen
	}

	switch node.identifier {
	case f_0, f_1:
		node.scriptLen = 1

	case f_pk_k:
		node.scriptLen = node.ctx.keyPushLen()

	case f_pk_h:
		node.scriptLen = 24

	case f_older, f_after:
		n := node.args[0].num
		node.scriptLen = 1 + numPushLen(int64(n))

	case f_sha256, f_hash256:
		node.scriptLen = 39

	case f_ripemd160, f_hash160:
		node.scriptLen = 27

	case f_andor, f_or_i, f_or_d, f_wrap_d:
		node.scriptLen = argsSummed + 3

	case f_and_v:
		node.scriptLen = argsSummed

	case f_and_b, f_or_b, f_wrap_s, f_wrap_c, f_wrap_n:
		node.scriptLen = argsSummed + 1

	case f_or_c, f_wrap_a:
		node.scriptLen = argsSummed + 2

	case f_thresh:
		k := node.args[0].num
		numSubs := len(node.args) - 1

		// The script is `sub_1 [sub_i OP_ADD](n-1 times) <k> OP_EQUAL`,
		// i.e. all sub expressions, (numSubs-1) OP_ADDs, the push of k,
		// and the final OP_EQUAL.
		node.scriptLen = argsSummed + (numSubs - 1) + numPushLen(
			int64(k),
		) + 1

	case f_multi:
		k := node.args[0].num
		numKeys := len(node.args) - 1
		node.scriptLen = numPushLen(int64(k)) +
			numKeys*node.ctx.keyPushLen() +
			numPushLen(int64(numKeys)) + 1

	case f_multi_a:
		k := node.args[0].num
		numKeys := len(node.args) - 1

		// The script is `<pk1> CHECKSIG <pk2> CHECKSIGADD ... <pkn>
		// CHECKSIGADD <k> NUMEQUAL`: n key pushes, one CHECKSIG plus
		// (n-1) CHECKSIGADDs, the push of k, and the final NUMEQUAL.
		node.scriptLen = numKeys*node.ctx.keyPushLen() +
			numKeys + numPushLen(int64(k)) + 1

	case f_wrap_v:
		if node.args[0].props.canCollapseVerify {
			// A VERIFY variant replaces the final opcode without
			// adding a byte (including NUMEQUALVERIFY for multi_a).
			node.scriptLen = argsSummed
		} else {
			node.scriptLen = argsSummed + 1
		}

	case f_wrap_j:
		node.scriptLen = argsSummed + 4

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// Script encodes the parsed expression in its script context. ApplyVars must
// first supply concrete key/hash values. The returned bytes are caller-owned.
func (a *AST) Script() ([]byte, error) {
	b := txscript.NewScriptBuilder()
	if err := buildScript(a, b, false); err != nil {
		return nil, err
	}
	return b.Script()
}

// buildScript builds the script from the tree. collapseVerify is true if a `v`
// wrapper (VERIFY wrapper) applies to the *final* opcode produced by this node.
// If so, and if that final opcode is OP_CHECKSIG, OP_EQUAL, OP_NUMEQUAL or
// OP_CHECKMULTISIG,
// it can be collapsed into the VERIFY variant (OP_CHECKSIGVERIFY,
// OP_EQUALVERIFY, OP_CHECKMULTISIGVERIFY) instead of emitting a separate
// OP_VERIFY.
//
// A `v:` wrapper only affects the single last opcode of its child (see
// rust-miniscript's `push_verify`), so collapseVerify must only be forwarded to
// the child that produces this node's final opcode: the second argument of
// and_v and the argument of the s: wrapper. Every other combinator's final
// opcode is a fixed, non-collapsible opcode (OP_BOOLAND, OP_BOOLOR, OP_ENDIF,
// OP_FROMALTSTACK, ...), so its children must be built with collapseVerify set
// to false, otherwise inner collapsible opcodes would be wrongly turned into
// their VERIFY variants and produce an invalid script.
func buildScript(node *AST, b *txscript.ScriptBuilder,
	collapseVerify bool) error {

	switch node.identifier {
	case f_0:
		b.AddOp(txscript.OP_FALSE)

	case f_1:
		b.AddOp(txscript.OP_TRUE)

	case f_pk_k:
		arg := node.args[0]
		key := arg.value
		if key == nil {
			return fmt.Errorf("empty key for %s (%s)",
				node.identifier, arg.identifier)
		}
		b.AddData(key)

	case f_pk_h:
		arg := node.args[0]
		key := arg.value
		if key == nil {
			return fmt.Errorf("empty key for %s (%s)",
				node.identifier, arg.identifier)
		}
		b.AddOp(txscript.OP_DUP)
		b.AddOp(txscript.OP_HASH160)
		b.AddData(address.Hash160(key))
		b.AddOp(txscript.OP_EQUALVERIFY)

	case f_older:
		b.AddInt64(int64(node.args[0].num))
		b.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)

	case f_after:
		b.AddInt64(int64(node.args[0].num))
		b.AddOp(txscript.OP_CHECKLOCKTIMEVERIFY)

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		hashOp := map[string]byte{
			f_sha256:    txscript.OP_SHA256,
			f_hash256:   txscript.OP_HASH256,
			f_ripemd160: txscript.OP_RIPEMD160,
			f_hash160:   txscript.OP_HASH160,
		}[node.identifier]

		hashValue := node.args[0].value
		if hashValue == nil {
			return fmt.Errorf("hash value empty for %s (%s)",
				node.identifier, node.args[0].identifier)
		}
		b.AddOp(txscript.OP_SIZE)
		b.AddInt64(32)
		b.AddOp(txscript.OP_EQUALVERIFY)
		b.AddOp(hashOp)
		b.AddData(hashValue)
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_EQUALVERIFY)
		} else {
			b.AddOp(txscript.OP_EQUAL)
		}

	case f_andor:
		// andor's final opcode is OP_ENDIF, so no child is the collapse
		// target.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[2], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ELSE)
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_and_v:
		// and_v emits [X][Y], so the final opcode is Y's final opcode:
		// forward collapseVerify to the second argument only.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}

	case f_and_b:
		// and_b's final opcode is OP_BOOLAND.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_BOOLAND)

	case f_or_b:
		// or_b's final opcode is OP_BOOLOR.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_BOOLOR)

	case f_or_c:
		// or_c's final opcode is OP_ENDIF.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_or_d:
		// or_d's final opcode is OP_ENDIF.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_IFDUP)
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_or_i:
		// or_i's final opcode is OP_ENDIF.
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ELSE)
		err = buildScript(node.args[1], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_thresh:
		k := node.args[0].num

		// The sub expressions never produce the final opcode (that is
		// the OP_EQUAL below), so they are built without collapse.
		for i, arg := range node.args[1:] {
			err := buildScript(arg, b, false)
			if err != nil {
				return err
			}
			if i > 0 {
				b.AddOp(txscript.OP_ADD)
			}
		}
		b.AddInt64(int64(k))
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_EQUALVERIFY)
		} else {
			b.AddOp(txscript.OP_EQUAL)
		}

	case f_multi:
		k := node.args[0].num
		b.AddInt64(int64(k))
		for _, arg := range node.args[1:] {
			if arg.value == nil {
				return fmt.Errorf("empty key for %s (%s)",
					node.identifier, arg.identifier)
			}
			b.AddData(arg.value)
		}
		b.AddInt64(int64(len(node.args) - 1))
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_CHECKMULTISIGVERIFY)
		} else {
			b.AddOp(txscript.OP_CHECKMULTISIG)
		}

	case f_multi_a:
		// multi_a emits `<pk1> CHECKSIG <pk2> CHECKSIGADD ... <pkn>
		// CHECKSIGADD <k> NUMEQUAL`. The final opcode is OP_NUMEQUAL,
		// which is collapsed into OP_NUMEQUALVERIFY under a v: wrapper.
		k := node.args[0].num
		for i, arg := range node.args[1:] {
			if arg.value == nil {
				return fmt.Errorf("empty key for %s (%s)",
					node.identifier, arg.identifier)
			}
			b.AddData(arg.value)
			if i == 0 {
				b.AddOp(txscript.OP_CHECKSIG)
			} else {
				b.AddOp(txscript.OP_CHECKSIGADD)
			}
		}
		b.AddInt64(int64(k))
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_NUMEQUALVERIFY)
		} else {
			b.AddOp(txscript.OP_NUMEQUAL)
		}

	case f_wrap_a:
		// a: emits OP_TOALTSTACK [X] OP_FROMALTSTACK, so the final
		// opcode is OP_FROMALTSTACK.
		b.AddOp(txscript.OP_TOALTSTACK)
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_FROMALTSTACK)

	case f_wrap_s:
		// s: emits OP_SWAP [X], so the final opcode is X's final
		// opcode: forward collapseVerify to the child.
		b.AddOp(txscript.OP_SWAP)
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}

	case f_wrap_c:
		// c: emits [X] OP_CHECKSIG; the final opcode is the OP_CHECKSIG
		// below, so the child is not the collapse target.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_CHECKSIGVERIFY)
		} else {
			b.AddOp(txscript.OP_CHECKSIG)
		}

	case f_wrap_d:
		// d: emits OP_DUP OP_IF [X] OP_ENDIF, so the final opcode is
		// OP_ENDIF.
		b.AddOp(txscript.OP_DUP)
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_wrap_v:
		if err := buildScript(node.args[0], b, true); err != nil {
			return err
		}
		if !node.args[0].props.canCollapseVerify {
			b.AddOp(txscript.OP_VERIFY)
		}

	case f_wrap_j:
		// j: emits OP_SIZE OP_0NOTEQUAL OP_IF [X] OP_ENDIF, so the
		// final opcode is OP_ENDIF.
		b.AddOp(txscript.OP_SIZE)
		b.AddOp(txscript.OP_0NOTEQUAL)
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_wrap_n:
		// n: emits [X] OP_0NOTEQUAL, so the final opcode is
		// OP_0NOTEQUAL.
		err := buildScript(node.args[0], b, false)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_0NOTEQUAL)

	default:
		return fmt.Errorf("unknown identifier: %s", node.identifier)
	}

	return nil
}

// scriptStr outputs a human-readable version of the script for debugging
// purposes. collapseVerify applies only to this node's final opcode, exactly
// as in buildScript; it is not propagated to every descendant of a v: wrapper.
func scriptStr(node *AST, collapseVerify bool) string {
	switch node.identifier {
	case f_0, f_1:
		return node.identifier

	case f_pk_k:
		return fmt.Sprintf("<%s>", node.args[0].identifier)

	case f_pk_h:
		return fmt.Sprintf("DUP HASH160 <HASH160(%s)> EQUALVERIFY",
			node.args[0].identifier)

	case f_older:
		return fmt.Sprintf("<%s> CHECKSEQUENCEVERIFY",
			node.args[0].identifier)

	case f_after:
		return fmt.Sprintf("<%s> CHECKLOCKTIMEVERIFY",
			node.args[0].identifier)

	case f_sha256, f_hash256, f_ripemd160, f_hash160:
		opVerify := "EQUAL"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "EQUALVERIFY"
		}
		return fmt.Sprintf("SIZE <32> EQUALVERIFY %s <%s> %s",
			strings.ToUpper(node.identifier),
			node.args[0].identifier, opVerify)

	case f_andor:
		return fmt.Sprintf("%s NOTIF %s ELSE %s ENDIF",
			scriptStr(node.args[0], false),
			scriptStr(node.args[2], false),
			scriptStr(node.args[1], false))

	case f_and_v:
		return fmt.Sprintf("%s %s",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], collapseVerify))

	case f_and_b:
		return fmt.Sprintf("%s %s BOOLAND",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], false))

	case f_or_b:
		return fmt.Sprintf("%s %s BOOLOR",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], false))

	case f_or_c:
		return fmt.Sprintf("%s NOTIF %s ENDIF",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], false))

	case f_or_d:
		return fmt.Sprintf("%s IFDUP NOTIF %s ENDIF",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], false))

	case f_or_i:
		return fmt.Sprintf("IF %s ELSE %s ENDIF",
			scriptStr(node.args[0], false),
			scriptStr(node.args[1], false))

	case f_thresh:
		var s []string
		for i, arg := range node.args[1:] {
			s = append(s, scriptStr(arg, false))
			if i > 0 {
				s = append(s, "ADD")
			}
		}

		opVerify := "EQUAL"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "EQUALVERIFY"
		}
		s = append(s, node.args[0].identifier)
		s = append(s, opVerify)
		return strings.Join(s, " ")

	case f_multi:
		s := []string{node.args[0].identifier}
		for _, arg := range node.args[1:] {
			s = append(s, fmt.Sprintf("<%s>", arg.identifier))
		}
		opVerify := "CHECKMULTISIG"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "CHECKMULTISIGVERIFY"
		}
		s = append(s, fmt.Sprint(len(node.args)-1))
		s = append(s, opVerify)
		return strings.Join(s, " ")

	case f_multi_a:
		var s []string
		for i, arg := range node.args[1:] {
			s = append(s, fmt.Sprintf("<%s>", arg.identifier))
			if i == 0 {
				s = append(s, "CHECKSIG")
			} else {
				s = append(s, "CHECKSIGADD")
			}
		}
		opVerify := "NUMEQUAL"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "NUMEQUALVERIFY"
		}
		s = append(s, node.args[0].identifier)
		s = append(s, opVerify)
		return strings.Join(s, " ")

	case f_wrap_a:
		return fmt.Sprintf("TOALTSTACK %s FROMALTSTACK",
			scriptStr(node.args[0], false))

	case f_wrap_s:
		return fmt.Sprintf("SWAP %s",
			scriptStr(node.args[0], collapseVerify))

	case f_wrap_c:
		opVerify := "CHECKSIG"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "CHECKSIGVERIFY"
		}
		return fmt.Sprintf("%s %s",
			scriptStr(node.args[0], false),
			opVerify)

	case f_wrap_d:
		return fmt.Sprintf("DUP IF %s ENDIF",
			scriptStr(node.args[0], false))

	case f_wrap_v:
		s := scriptStr(node.args[0], true)
		if !node.args[0].props.canCollapseVerify {
			s += " VERIFY"
		}
		return s

	case f_wrap_j:
		return fmt.Sprintf("SIZE 0NOTEQUAL IF %s ENDIF",
			scriptStr(node.args[0], false))

	case f_wrap_n:
		return fmt.Sprintf("%s 0NOTEQUAL",
			scriptStr(node.args[0], false))

	default:
		return "<unknown>"
	}
}

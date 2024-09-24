package miniscript

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	// pubKeyLen is the length of a public key inside P2WSH, which are 33
	// byte compressed public keys.
	pubKeyLen = 33

	// pubKeyDataPushLen is the length of a public key data push in P2WSH,
	// which is 1+33 (1 byte for the VarInt encoding of 33).
	pubKeyDataPushLen = 34

	// maxStandardP2WSHScriptSize is he maximum size in bytes of a standard
	// witnessScript.
	maxStandardP2WSHScriptSize = 3600

	// maxOpsPerScript is the maximum number of non-push operations per
	// script.
	maxOpsPerScript = 201

	// multisigMaxKeys is the maximum number of keys in a multisig.
	multisigMaxKeys = 20
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
	f_multi     = "multi"     // multi(k,key1,...,keyn)
	f_wrap_a    = "a"         // a:X
	f_wrap_s    = "s"         // s:X
	f_wrap_c    = "c"         // c:X
	f_wrap_d    = "d"         // d:X
	f_wrap_v    = "v"         // v:X
	f_wrap_j    = "j"         // j:X
	f_wrap_n    = "n"         // n:X
	f_wrap_t    = "t"         // t:X = and_v(X,1)
	f_wrap_l    = "l"         // l:X = or_i(0,X)
	f_wrap_u    = "u"         // u:X = or_i(X,0)
)

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

func (p properties) String() string {
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

// Parse a miniscript expression, assuming it will be executed in P2WSH. The
// resulting node is checked to be a valid base expression (type "B") and that
// the script size does not exceed 3600 bytes, which is a standardness rule.
//
// The following transformations are applied to the AST in order:
//  1. argCheck: Checks that the nodes have the correct number of arguments.
//  2. expandWrappers: Unwraps the numbers before the colon, for example:
//     dv:older(144) is d(v(older(144)))
//  3. deSugar: Miniscript defines six instances of syntactic sugar. We replace
//     these with fixed equations.
//  4. typeCheck: Not all fragments compose with each other to produce a valid
//     Bitcoin Script and valid witness. This function checks that and sets the
//     types of the Miniscript fragments. Only if the top level basic type is of
//     type B the miniscript is valid.
//  5. canCollapseVerify: If the rightmost script byte of a node is OP_EQUAL,
//     OP_CHECKSIG or OP_CHECKMULTISIG. We can convert it to the VERIFY version
//     of the opcode, e.g. OP_EQUALVERIFY.
//  6. malleabilityCheck: Checks each node if it is malleable (checking that the
//     transaction hash can not be changes without altering the content).
//  7. computeScriptLen: Simply computes the script length.
//  8. computeOpCount: Counts the amount of opcodes the script contains.
func Parse(miniscript string) (*AST, error) {
	node, err := createAST(miniscript)
	if err != nil {
		return nil, err
	}

	transformers := []func(*AST) (*AST, error){
		argCheck,
		expandWrappers,
		deSugar,
		typeCheck,
		canCollapseVerify,
		malleabilityCheck,
		computeScriptLen,
		computeOpCount,
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
	basicType  basicType
	props      properties
	wrappers   string
	identifier string

	// num is the parsed integer for when identifier is expected to be a
	// number, i.e. the first argument of older/after/multi/thresh. This is
	// not used otherwise.
	num uint64

	// For key arguments, this will be the 33 bytes compressed pubkey.
	// For hash arguments, this will be the 32 bytes (sha256, hash256) or
	// 20 bytes (ripemd160, hash160) hash.
	value     []byte
	args      []*AST
	scriptLen int
	opCount   ops
}

// formattedType returns the basic type (B, V, K or W) followed by all type
// properties.
func (a *AST) formattedType() string {
	return fmt.Sprintf("%s%s", a.basicType, a.props)
}

func (a *AST) isValid() error {
	if a.scriptLen > maxStandardP2WSHScriptSize {
		return fmt.Errorf("the script size is %v, which is larger "+
			"than the maximum standard P2WSH script size of %v",
			a.scriptLen, maxStandardP2WSHScriptSize)
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
// guaranteed to be valid and that a satisfaction does not violate the maximum
// stack size.
func (a *AST) validSatisfactions() error {
	if err := a.isValid(); err != nil {
		return err
	}
	if a.maxOpCount() > maxOpsPerScript {
		return fmt.Errorf("the script requires a maximum number of %d "+
			"ops, which is larger than the consensus limit of %d",
			a.maxOpCount(), maxOpsPerScript)
	}

	// TODO(guggero): check stack size
	return nil
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

	// TODO(guggero): check timelock mixing, see
	// https://medium.com/blockstream/dont-mix-your-timelocks-d9939b665094.
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

func (a *AST) drawTree(w io.Writer, indent string) {
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
		padLen := len([]rune(arg.identifier)) + len([]rune(mark)) -
			1 - len(delim)
		padding := strings.Repeat(" ", padLen)
		arg.drawTree(w, indent+delim+padding)
	}
}

func (a *AST) DrawTree() string {
	var b strings.Builder
	a.drawTree(&b, "")
	return b.String()
}

func (a *AST) apply(f func(*AST) (*AST, error)) (*AST, error) {
	for i, arg := range a.args {
		// We don't recurse into arguments which are not miniscript
		// subexpressions themselves:
		// key/hash variables and the numeric arguments of
		// older/after/multi/thresh.
		switch a.identifier {
		case f_pk_k, f_pk_h, f_pk, f_pkh,
			f_sha256, f_hash256, f_ripemd160, f_hash160,
			f_older, f_after, f_multi:

			// None of the arguments of these functions are
			// miniscript subexpressions - they are
			// variables (or concrete assignments) or numbers.
			continue

		case f_thresh:
			// First argument is a number. The other arguments are
			// subexpressions, which we want to visit, so only skip
			// the first argument.
			if i == 0 {
				continue
			}
		}

		newArg, err := arg.apply(f)
		if err != nil {
			return nil, err
		}
		a.args[i] = newArg
	}
	return f(a)
}

// ApplyVars replaces key and hash values in the miniscript. It must be called
// before running Script() or Satisfy().
//
// The callback should return `nil, nil` if the variable is unknown. In this
// case, the identifier itself will be parsed as the value (hex-encoded pubkey,
// hex-encoded hash value).
func (a *AST) ApplyVars(lookupVar func(identifier string) ([]byte, error)) error {
	// Set of all pubkeys to check for duplicates
	allPubKeys := map[string]struct{}{}

	_, err := a.apply(func(node *AST) (*AST, error) {
		switch node.identifier {
		case f_pk_k, f_pk_h, f_multi:
			var keyArgs []*AST
			if node.identifier == f_multi {
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
				if len(key) != pubKeyLen {
					return nil, fmt.Errorf("pubkey "+
						"argument of %s expected to "+
						"be of size %d, but got %d",
						node.identifier, pubKeyLen,
						len(key))
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

type stack struct {
	elements []*AST
}

func (s *stack) push(element *AST) {
	s.elements = append(s.elements, element)
}

func (s *stack) pop() *AST {
	if len(s.elements) == 0 {
		return nil
	}
	top := s.elements[len(s.elements)-1]
	s.elements = s.elements[:len(s.elements)-1]
	return top
}

func (s *stack) top() *AST {
	if len(s.elements) == 0 {
		return nil
	}
	return s.elements[len(s.elements)-1]
}

func (s *stack) size() int {
	return len(s.elements)
}

// splitString keeps separators as individual slice elements and splits a string
// into a slice of strings based on multiple separators. It removes any empty
// elements from the output slice.
// - Written by ChatGPT.
func splitString(s string, isSeparator func(c rune) bool) []string {
	// Create a slice to hold the substrings.
	substrings := make([]string, 0)

	// Set the initial index to zero.
	i := 0

	// Iterate over the characters in the string.
	for i < len(s) {
		// Find the index of the first separator in the string.
		j := strings.IndexFunc(s[i:], isSeparator)
		if j == -1 {
			// If no separator was found, append the remaining
			// substring and return.
			substrings = append(substrings, s[i:])
			return substrings
		}
		j += i

		// If a separator was found, append the substring before it.
		if j > i {
			substrings = append(substrings, s[i:j])
		}

		// Append the separator as a separate element.
		substrings = append(substrings, s[j:j+1])
		i = j + 1
	}
	return substrings
}

func createAST(miniscript string) (*AST, error) {
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

	// Build abstract syntax tree.
	var stack stack
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
			// parent, the expression is unbalanced, e.g. `f(X))``.
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
			// identifier.
			var (
				parts                = strings.Split(token, ":")
				wrappers, identifier string
			)
			if len(parts) == 1 {
				// No colon => Only an identifier.
				identifier = parts[0]
			} else if len(parts) == 2 {
				wrappers, identifier = parts[0], parts[1]

				if wrappers == "" {
					return nil, fmt.Errorf("no wrappers "+
						"found before colon before "+
						"identifier: %s", identifier)
				} else if identifier == "" {
					return nil, fmt.Errorf("no identifier "+
						"found after colon after "+
						"wrappers: %s", wrappers)
				}
			} else {
				return nil, fmt.Errorf("invalid number of "+
					"colons in token: %s", token)
			}

			stack.push(&AST{
				wrappers:   wrappers,
				identifier: identifier,
			})
		}
	}

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
		if len(node.args[0].args) > 0 {
			return nil, fmt.Errorf("argument of %s must not "+
				"contain subexpressions", node.identifier)
		}

	case f_older, f_after:
		if err := expectArgs(1); err != nil {
			return nil, err
		}
		_n := node.args[0]
		if len(_n.args) > 0 {
			return nil, fmt.Errorf("argument of %s must not "+
				"contain subexpressions", node.identifier)
		}
		n, err := strconv.ParseUint(_n.identifier, 10, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"%s(k) => k must be an unsigned integer, but "+
					"got: %s", node.identifier,
				_n.identifier)
		}
		_n.num = n
		if n < 1 || n >= (1<<31) {
			return nil, fmt.Errorf("%s(n) -> n must 1 ≤ n < 2^31, "+
				"but got: %s", node.identifier, _n.identifier)
		}

	case f_andor:
		if err := expectArgs(3); err != nil {
			return nil, err
		}

	case f_and_v, f_and_b, f_and_n, f_or_b, f_or_c, f_or_d, f_or_i:
		if err := expectArgs(2); err != nil {
			return nil, err
		}

	case f_thresh, f_multi:
		if len(node.args) < 2 {
			return nil, fmt.Errorf("%s must have at least two "+
				"arguments", node.identifier)
		}
		_k := node.args[0]
		if len(_k.args) > 0 {
			return nil, fmt.Errorf("argument of %s must not "+
				"contain subexpressions", node.identifier)
		}
		k, err := strconv.ParseUint(_k.identifier, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s(k, ...) => k must be an "+
				"integer, but got: %s", node.identifier,
				_k.identifier)
		}
		_k.num = k
		numSubs := len(node.args) - 1
		if k < 1 || k > uint64(numSubs) {
			return nil, fmt.Errorf("%s(k) -> k must 1 ≤ k ≤ n, "+
				"but got: %s", node.identifier, _k.identifier)
		}
		if node.identifier == f_multi {
			if numSubs > multisigMaxKeys {
				return nil, fmt.Errorf("number of multisig "+
					"keys cannot exceed %d",
					multisigMaxKeys)
			}
			// Multisig keys are variables, they can't have
			// subexpressions.
			for _, arg := range node.args {
				if len(arg.args) > 0 {
					return nil, fmt.Errorf("arguments of "+
						"%s must not contain "+
						"subexpressions",
						node.identifier)
				}
			}
		}

	default:
		return nil, fmt.Errorf("unrecognized identifier: %s",
			node.identifier)
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

func typeCheck(node *AST) (*AST, error) {
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
		_x, _y, _z := node.args[0], node.args[1], node.args[2]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.d || !_x.props.u {
			return nil, fmt.Errorf("wrong properties on `%s` in "+
				"the first argument of `%s`", _x.identifier,
				node.identifier)
		}
		if _y.basicType != typeB && _y.basicType != typeK &&
			_y.basicType != typeV {

			return nil, fmt.Errorf("in `%s`, the second argument "+
				"type is not B, K or V, but: %s",
				node.identifier, _y.basicType)
		}
		if _z.basicType != _y.basicType {
			return nil, fmt.Errorf("in `%s`, the third of the "+
				"argument is not the same as the type of the "+
				"second argument, which is: %s",
				node.identifier, _y.basicType)
		}
		node.basicType = _y.basicType
		node.props.z = _x.props.z && _y.props.z && _z.props.z
		node.props.o = (_x.props.z && _y.props.o && _z.props.o) ||
			(_x.props.o && _y.props.z && _z.props.z)
		node.props.u = _y.props.u && _z.props.u
		node.props.d = _z.props.d

	case f_and_v:
		_x, _y := node.args[0], node.args[1]
		if err := _x.expectBasicType(typeV); err != nil {
			return nil, err
		}
		if _y.basicType != typeB && _y.basicType != typeK &&
			_y.basicType != typeV {

			return nil, fmt.Errorf("in `%s`, the second argument "+
				"type is not B, K or V, but: %s",
				node.identifier, _y.basicType)
		}
		node.basicType = _y.basicType
		node.props.z = _x.props.z && _y.props.z
		node.props.o = (_x.props.z && _y.props.o) ||
			(_y.props.z && _x.props.o)
		node.props.n = _x.props.n || (_x.props.z && _y.props.n)
		node.props.u = _y.props.u

	case f_and_b:
		_x, _y := node.args[0], node.args[1]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if err := _y.expectBasicType(typeW); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = _x.props.z && _y.props.z
		node.props.o = (_x.props.z && _y.props.o) ||
			(_y.props.z && _x.props.o)
		node.props.n = _x.props.n || (_x.props.z && _y.props.n)
		node.props.d = _x.props.d && _y.props.d
		node.props.u = true

	case f_or_b:
		_x, _z := node.args[0], node.args[1]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.d {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", _x.identifier,
				node.identifier)
		}
		if err := _z.expectBasicType(typeW); err != nil {
			return nil, err
		}
		if !_z.props.d {
			return nil, fmt.Errorf(
				"wrong properties on `%s`, the second "+
					"argument of `%s`", _z.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.z = _x.props.z && _z.props.z
		node.props.o = (_x.props.z && _z.props.o) ||
			(_z.props.z && _x.props.o)
		node.props.d = true
		node.props.u = true

	case f_or_c:
		_x, _z := node.args[0], node.args[1]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.d || !_x.props.u {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", _x.identifier,
				node.identifier)
		}
		if err := _z.expectBasicType(typeV); err != nil {
			return nil, err
		}
		node.basicType = typeV
		node.props.z = _x.props.z && _z.props.z
		node.props.o = _x.props.o && _z.props.z

	case f_or_d:
		_x, _z := node.args[0], node.args[1]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.d || !_x.props.u {
			return nil, fmt.Errorf(
				"wrong properties on `%s`, the first argument "+
					"of `%s`", _x.identifier,
				node.identifier)
		}
		if err := _z.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = _x.props.z && _z.props.z
		node.props.o = _x.props.o && _z.props.z
		node.props.d = _z.props.d
		node.props.u = _z.props.u

	case f_or_i:
		_x, _z := node.args[0], node.args[1]
		if _x.basicType != typeB && _x.basicType != typeK &&
			_x.basicType != typeV {

			return nil, errors.New("or_i: wrong type of first " +
				"argument")
		}
		if _z.basicType != _x.basicType {
			return nil, errors.New("or_i: wrong type of second " +
				"argument")
		}
		node.basicType = _x.basicType
		node.props.o = _x.props.z && _z.props.z
		node.props.u = _x.props.u && _z.props.u
		node.props.d = _x.props.d || _z.props.d

	case f_thresh:
		//  X1 is Bdu; others are Wdu
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
		// z=all are z; o=all are z except one is o; d; u
		node.props.z = true
		node.props.o = true
		for _, arg := range node.args[1:] {
			node.props.z = node.props.z && arg.props.z
			node.props.o = node.props.o && arg.props.z &&
				!(arg.props.o || arg.props.d || arg.props.u)
		}
		node.props.d = true
		node.props.u = true

	case f_multi:
		node.basicType = typeB
		node.props.n = true
		node.props.d = true
		node.props.u = true

	case f_wrap_a:
		_x := node.args[0]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeW
		node.props.d = _x.props.d
		node.props.u = _x.props.u

	case f_wrap_s:
		_x := node.args[0]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.o {
			return nil, fmt.Errorf("wrong properties on `%s`, the "+
				"first argument of `%s`", _x.identifier,
				node.identifier)
		}
		node.basicType = typeW
		node.props.d = _x.props.d
		node.props.u = _x.props.u

	case f_wrap_c:
		_x := node.args[0]
		if err := _x.expectBasicType(typeK); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.o = _x.props.o
		node.props.n = _x.props.n
		node.props.d = _x.props.d
		node.props.u = true

	case f_wrap_d:
		_x := node.args[0]
		if err := _x.expectBasicType(typeV); err != nil {
			return nil, err
		}
		if !_x.props.z {
			return nil, fmt.Errorf("wrong property of `%s`, the "+
				"first argument of `%s`", _x.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.o = true
		node.props.n = true
		node.props.d = true

	case f_wrap_v:
		_x := node.args[0]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeV
		node.props.z = _x.props.z
		node.props.o = _x.props.o
		node.props.n = _x.props.n

	case f_wrap_j:
		_x := node.args[0]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		if !_x.props.n {
			return nil, fmt.Errorf("wrong property of `%s`, the "+
				"first argument of `%s`", _x.identifier,
				node.identifier)
		}
		node.basicType = typeB
		node.props.o = _x.props.o
		node.props.n = true
		node.props.d = true
		node.props.u = _x.props.u

	case f_wrap_n:
		_x := node.args[0]
		if err := _x.expectBasicType(typeB); err != nil {
			return nil, err
		}
		node.basicType = typeB
		node.props.z = _x.props.z
		node.props.o = _x.props.o
		node.props.n = _x.props.n
		node.props.d = _x.props.d
		node.props.u = true

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}
	return node, nil
}

func canCollapseVerify(node *AST) (*AST, error) {
	switch node.identifier {
	case f_sha256, f_ripemd160, f_hash256, f_hash160, f_thresh, f_multi,
		f_wrap_c:

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

func malleabilityCheck(node *AST) (*AST, error) {
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
		_x, _y := node.args[0].props, node.args[1].props
		_z := node.args[2].props
		node.props.m = _x.m && _y.m && _z.m &&
			(_x.e && (_x.s || _y.s || _z.s))
		node.props.s = _z.s && (_x.s || _y.s)
		node.props.f = _z.f && (_x.s || _y.f)
		node.props.e = _z.e && (_x.s || _y.f)

	case f_and_v:
		_x, _y := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _y.m
		node.props.s = _x.s || _y.s
		node.props.f = _x.s || _y.f

	case f_and_b:
		_x, _y := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _y.m
		node.props.s = _x.s || _y.s
		node.props.f = _x.f && _y.f || _x.s && _x.f || _y.s && _y.f
		node.props.e = _x.e && _y.e && _x.s && _y.s

	case f_or_b:
		_x, _z := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _z.m && (_x.e && _z.e && (_x.s || _z.s))
		node.props.s = _x.s && _z.s
		node.props.e = true

	case f_or_c:
		_x, _z := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _z.m && (_x.e && (_x.s || _z.s))
		node.props.s = _x.s && _z.s
		node.props.f = true

	case f_or_d:
		_x, _z := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _z.m && (_x.e && (_x.s || _z.s))
		node.props.s = _x.s && _z.s
		node.props.f = _z.f

		// Note: the implementation at https://github.com/sipa/miniscript/ uses
		// `e=e_x*e_z`:
		// https://github.com/sipa/miniscript/blob/484386a50dbda962669cc163f239fe16e101b6f0/bitcoin/script/miniscript.cpp#L175
		// while the specification and rust-miniscript both use `e=e_z`:
		// - https://github.com/sipa/miniscript/blob/484386a50dbda962669cc163f239fe16e101b6f0/index.html#L624
		// - https://github.com/rust-bitcoin/rust-miniscript/blob/a0648b3a4d63abbe53f621308614f97f04a04096/src/miniscript/types/malleability.rs#L241
		// In case of `m==false` (all satisfactions are malleable), `e`
		// can have a different type in each of the two possible
		// implementations, but it does not matter, as its purpose is to
		// compute `m`.
		// See https://github.com/sipa/miniscript/issues/128
		node.props.e = _z.e

	case f_or_i:
		_x, _z := node.args[0].props, node.args[1].props
		node.props.m = _x.m && _z.m && (_x.s || _z.s)
		node.props.s = _x.s && _z.s
		node.props.f = _x.f && _z.f
		node.props.e = _x.e && _z.f || _z.e && _x.f

	case f_thresh:
		k := node.args[0].num
		notSCount := 0
		node.props.m = true
		for _, arg := range node.args[1:] {
			node.props.m = node.props.m && arg.props.m && arg.props.e
			if !arg.props.s {
				notSCount++
			}
		}
		node.props.m = node.props.m && uint64(notSCount) <= k
		node.props.s = uint64(notSCount) <= k-1
		node.props.e = true
		for _, arg := range node.args[1:] {
			node.props.e = node.props.e && arg.props.e && arg.props.s
		}

	case f_multi:
		node.props.m = true
		node.props.s = true
		node.props.e = true
	case f_wrap_a, f_wrap_s:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = _x.s
		node.props.f = _x.f
		node.props.e = _x.e

	case f_wrap_c:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = true
		node.props.f = _x.f
		node.props.e = _x.e

	case f_wrap_d:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = _x.s
		node.props.e = true

	case f_wrap_v:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = _x.s
		node.props.f = true

	case f_wrap_j:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = _x.s
		node.props.e = _x.f

	case f_wrap_n:
		_x := node.args[0].props
		node.props.m = _x.m
		node.props.s = _x.s
		node.props.f = _x.f
		node.props.e = _x.e

	default:
		return nil, fmt.Errorf("unknown identifier: %s",
			node.identifier)
	}

	return node, nil
}

// Compute the length of the resulting witness script.
func computeScriptLen(node *AST) (*AST, error) {
	numPushLen := func(n int64) int {
		numPush, _ := txscript.NewScriptBuilder().AddInt64(n).Script()
		return len(numPush)
	}
	argsSummed := 0
	for _, arg := range node.args {
		argsSummed += arg.scriptLen
	}

	switch node.identifier {
	case f_0, f_1:
		node.scriptLen = 1

	case f_pk_k:
		node.scriptLen = pubKeyDataPushLen

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
		node.scriptLen = argsSummed + int(k) + 1 + numPushLen(int64(k))

	case f_multi:
		k := node.args[0].num
		numKeys := len(node.args) - 1
		node.scriptLen = numPushLen(int64(k)) +
			numKeys*pubKeyDataPushLen +
			numPushLen(int64(numKeys)) + 1

	case f_wrap_v:
		if node.args[0].props.canCollapseVerify {
			// OP_VERIFY not needed, collapsed into OP_EQUALVERIfY,
			// OP_CHECKSIGVERIFY, OP_CHECKMULTISIGVERIFY
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

// Script creates the witness script from a parsed miniscript.
func (a *AST) Script() ([]byte, error) {
	b := txscript.NewScriptBuilder()
	if err := buildScript(a, b, false); err != nil {
		return nil, err
	}
	return b.Script()
}

// buildScript builds the script from the tree. collapseVerify is true if the
// `v` wrapper (VERIFY wrapper) is an ancestor of the node. If so, the two
// opcodes `OP_CHECKSIG VERIFY` can be collapsed into one opcode
// `OP_CHECKSIGVERICY` (same for OP_EQUAL and OP_CHECKMULTISIGVERIFY).
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
		b.AddData(btcutil.Hash160(key))
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
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[2], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ELSE)
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_and_v:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}

	case f_and_b:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_BOOLAND)

	case f_or_b:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_BOOLOR)

	case f_or_c:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_or_d:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_IFDUP)
		b.AddOp(txscript.OP_NOTIF)
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_or_i:
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ELSE)
		err = buildScript(node.args[1], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_thresh:
		k := node.args[0].num

		for i := 1; i < len(node.args); i++ {
			err := buildScript(node.args[i], b, collapseVerify)
			if err != nil {
				return err
			}
			if i > 1 {
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

	case f_wrap_a:
		b.AddOp(txscript.OP_TOALTSTACK)
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_FROMALTSTACK)

	case f_wrap_s:
		b.AddOp(txscript.OP_SWAP)
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}

	case f_wrap_c:
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		if node.props.canCollapseVerify && collapseVerify {
			b.AddOp(txscript.OP_CHECKSIGVERIFY)
		} else {
			b.AddOp(txscript.OP_CHECKSIG)
		}

	case f_wrap_d:
		b.AddOp(txscript.OP_DUP)
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, collapseVerify)
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
		b.AddOp(txscript.OP_SIZE)
		b.AddOp(txscript.OP_0NOTEQUAL)
		b.AddOp(txscript.OP_IF)
		err := buildScript(node.args[0], b, collapseVerify)
		if err != nil {
			return err
		}
		b.AddOp(txscript.OP_ENDIF)

	case f_wrap_n:
		err := buildScript(node.args[0], b, collapseVerify)
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
// purposes. collapseVerify is true if the `v` wrapper (VERIFY wrapper) is an
// ancestor of the node. If so, the two opcodes `OP_CHECKSIG VERIFY` can be
// collapsed into one opcode `OP_CHECKSIGVERIFY` (same for OP_EQUAL and
// OP_CHECKMULTISIG).
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
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[2], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_and_v:
		return fmt.Sprintf("%s %s",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_and_b:
		return fmt.Sprintf("%s %s BOOLAND",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_or_b:
		return fmt.Sprintf("%s %s BOOLOR",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_or_c:
		return fmt.Sprintf("%s NOTIF %s ENDIF",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_or_d:
		return fmt.Sprintf("%s IFDUP NOTIF %s ENDIF",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_or_i:
		return fmt.Sprintf("IF %s ELSE %s ENDIF",
			scriptStr(node.args[0], collapseVerify),
			scriptStr(node.args[1], collapseVerify))

	case f_thresh:
		var s []string
		for i := 1; i < len(node.args); i++ {
			s = append(s, scriptStr(node.args[i], collapseVerify))
			if i > 1 {
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

	case f_wrap_a:
		return fmt.Sprintf("TOALTSTACK %s FROMALTSTACK",
			scriptStr(node.args[0], collapseVerify))

	case f_wrap_s:
		return fmt.Sprintf("SWAP %s",
			scriptStr(node.args[0], collapseVerify))

	case f_wrap_c:
		opVerify := "CHECKSIG"
		if node.props.canCollapseVerify && collapseVerify {
			opVerify = "CHECKSIGVERIFY"
		}
		return fmt.Sprintf("%s %s",
			scriptStr(node.args[0], collapseVerify),
			opVerify)

	case f_wrap_d:
		return fmt.Sprintf("DUP IF %s ENDIF",
			scriptStr(node.args[0], collapseVerify))

	case f_wrap_v:
		s := scriptStr(node.args[0], true)
		if !node.args[0].props.canCollapseVerify {
			s += " VERIFY"
		}
		return s

	case f_wrap_j:
		return fmt.Sprintf("SIZE 0NOTEQUAL IF %s ENDIF",
			scriptStr(node.args[0], collapseVerify))

	case f_wrap_n:
		return fmt.Sprintf("%s 0NOTEQUAL",
			scriptStr(node.args[0], collapseVerify))

	default:
		return "<unknown>"
	}
}

// Satisfy returns a valid non-malleable witness for this miniscript, given the
// available secrets (private keys and hash preimages). If no such witness could
// be found, an error is returned.
//
// The witness returned is a list of witness elements, each of which should be
// pushed onto the witness stack as a data push.
func (a *AST) Satisfy(satisfier *Satisfier) (wire.TxWitness, error) {
	satisfactions, err := satisfy(a, satisfier)
	if err != nil {
		return nil, err
	}
	if !satisfactions.sat.available {
		return nil, errors.New("no satisfaction could be found")
	}
	return satisfactions.sat.witness, nil
}

package descriptors

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/descriptors/miniscript"
)

const (
	// maxMultisigKeys is the maximum number of public keys
	// OP_CHECKMULTISIG accepts. This is a consensus rule, so a script with
	// more of them can never be satisfied.
	maxMultisigKeys = 20

	// maxBareMultisigKeys is the maximum number of public keys a bare
	// multisig may have to be standard (BIP383).
	maxBareMultisigKeys = 3

	// maxTapTreeDepth is the maximum depth of a taproot script tree. The
	// control block of a leaf holds one 32-byte hash per level of its
	// merkle path, and BIP341 allows at most 128 of them, so the leaves of
	// a deeper tree could never be spent.
	maxTapTreeDepth = 128
)

// nodeKind identifies the kind of a parsed descriptor tree node.
type nodeKind string

const (
	// nodePk is a bare pay-to-public-key: pk(KEY).
	nodePk nodeKind = "pk"

	// nodePkh is a pay-to-public-key-hash: pkh(KEY).
	nodePkh nodeKind = "pkh"

	// nodeWpkh is a pay-to-witness-public-key-hash: wpkh(KEY).
	nodeWpkh nodeKind = "wpkh"

	// nodeSh is a pay-to-script-hash wrapper: sh(INNER).
	nodeSh nodeKind = "sh"

	// nodeWsh is a pay-to-witness-script-hash wrapper: wsh(INNER).
	nodeWsh nodeKind = "wsh"

	// nodeTr is a pay-to-taproot output: tr(KEY) or tr(KEY, LEAF).
	nodeTr nodeKind = "tr"

	// nodeMulti is a multisig using OP_CHECKMULTISIG: multi(k, KEYS).
	nodeMulti nodeKind = "multi"

	// nodeSortedMulti is a multisig whose keys are sorted (BIP67) before
	// the script is built: sortedmulti(k, KEYS).
	nodeSortedMulti nodeKind = "sortedmulti"

	// nodeMs is a miniscript expression (the inner script of a wsh/sh, or a
	// taproot leaf).
	nodeMs nodeKind = "ms"
)

// scriptPos is the position a descriptor sub-expression appears in. It
// determines which public key serializations the descriptor BIPs allow there,
// and which miniscript context an inner miniscript expression is compiled in.
type scriptPos uint8

const (
	// posTop is the top level of a descriptor, i.e. a bare script.
	posTop scriptPos = iota

	// posSh is inside an sh() wrapper, i.e. a P2SH redeem script.
	posSh

	// posWsh is inside a wsh() wrapper, i.e. a segwit v0 witness script.
	posWsh

	// posTr is inside a tr() expression, i.e. the internal key or a
	// tapscript leaf.
	posTr
)

// String returns a description of the position, for use in an error message.
func (p scriptPos) String() string {
	switch p {
	case posSh:
		return "inside an sh() descriptor"

	case posWsh:
		return "inside a wsh() descriptor"

	case posTr:
		return "inside a tr() descriptor"

	default:
		return "at the top level of a descriptor"
	}
}

// isDescriptorExpr returns whether the kind is one of the descriptor script
// expressions, as opposed to a miniscript expression, whose name is a
// miniscript fragment identifier.
func (k nodeKind) isDescriptorExpr() bool {
	switch k {
	case nodePk, nodePkh, nodeWpkh, nodeSh, nodeWsh, nodeTr, nodeMulti,
		nodeSortedMulti:

		return true

	default:
		return false
	}
}

// allowedIn returns whether an expression of this kind may appear in the given
// position, following the BIPs that define each of them:
//
//   - pk() may be used in any position (BIP381).
//   - pkh(), multi() and sortedmulti() may be used at the top level, inside an
//     sh() or inside a wsh() (BIP381, BIP383).
//   - wpkh() and wsh() may be used at the top level or inside an sh() (BIP382).
//   - sh() and tr() may only be used at the top level (BIP381, BIP386).
//
// A miniscript expression is not covered here: parseMs decides which positions
// it may appear in, and its own context restricts it further.
func (k nodeKind) allowedIn(pos scriptPos) bool {
	switch k {
	case nodePk:
		return true

	case nodePkh, nodeMulti, nodeSortedMulti:
		return pos == posTop || pos == posSh || pos == posWsh

	case nodeWpkh, nodeWsh:
		return pos == posTop || pos == posSh

	case nodeSh, nodeTr:
		return pos == posTop

	default:
		return true
	}
}

// keyForm returns the serialization the keys of a pk/pkh/multi/sortedmulti in
// this position have to use. The bare and P2SH positions are pre-segwit, where
// BIP380/381/383 allow both compressed and uncompressed keys.
func (p scriptPos) keyForm() keyForm {
	switch p {
	case posWsh:
		return keyFormCompressed

	case posTr:
		return keyFormXOnly

	default:
		return keyFormLegacy
	}
}

// node is a parsed descriptor tree node.
type node struct {
	kind nodeKind

	// pos is the position the node appears in, which determines the key
	// serializations that are valid in it.
	pos scriptPos

	// keys holds the key(s) directly owned by this node: the single key of
	// pk/pkh/wpkh, the internal key of tr, or the key set of
	// multi/sortedmulti.
	keys []*descKey

	// thresh is the required signature count of multi/sortedmulti.
	thresh int

	// sub is the inner node of an sh or wsh wrapper.
	sub *node

	// tapTree is the taproot script tree of a tr output, or nil for a
	// key-path-only tr.
	tapTree *tapTree

	// msExpr and msCtx describe a miniscript node: its expression string
	// and the context (P2WSH, P2TR or Legacy) it is compiled in.
	msExpr string
	msCtx  miniscript.Context

	// msAST is the miniscript expression parsed once at descriptor
	// construction time. Since parsing (and its analysis pipeline) does not
	// depend on the concrete key values, it is cached here and cloned on
	// demand rather than re-parsed for every derivation, script build or
	// satisfaction. It is only set for nodeMs nodes.
	msAST *miniscript.AST
}

// clonedMsAST returns a fresh, mutable copy of the node's cached miniscript
// AST, ready for ApplyVars/Satisfy without affecting the cached tree.
func (n *node) clonedMsAST() *miniscript.AST {
	return n.msAST.Clone()
}

// DescType is the descriptor type.
type DescType string

const (
	// DescTypeBare is a bare descriptor (P2PK or bare multisig).
	DescTypeBare DescType = "Bare"

	// DescTypeSh is a pure P2SH descriptor (not nesting Wsh/Wpkh).
	DescTypeSh DescType = "Sh"

	// DescTypePkh is a P2PKH descriptor.
	DescTypePkh DescType = "Pkh"

	// DescTypeWpkh is a P2WPKH descriptor.
	DescTypeWpkh DescType = "Wpkh"

	// DescTypeWsh is a P2WSH descriptor.
	DescTypeWsh DescType = "Wsh"

	// DescTypeShWsh is a P2SH-wrapped P2WSH descriptor.
	DescTypeShWsh DescType = "ShWsh"

	// DescTypeShWpkh is a P2SH-wrapped P2WPKH descriptor.
	DescTypeShWpkh DescType = "ShWpkh"

	// DescTypeTr is a P2TR descriptor.
	DescTypeTr DescType = "Tr"
)

// DescType returns the descriptor type.
func (d *Descriptor) DescType() DescType {
	switch d.root.kind {
	case nodeTr:
		return DescTypeTr

	case nodePkh:
		return DescTypePkh

	case nodeWpkh:
		return DescTypeWpkh

	case nodeWsh:
		return DescTypeWsh

	case nodeSh:
		switch d.root.sub.kind {
		case nodeWpkh:
			return DescTypeShWpkh

		case nodeWsh:
			return DescTypeShWsh

		default:
			return DescTypeSh
		}

	default:
		// A top-level pk(), multi() or sortedmulti().
		return DescTypeBare
	}
}

// tapTree is a node of a taproot script tree: either a leaf holding a tapscript
// miniscript, or a branch with two children. It mirrors the BIP386 tr() tree
// syntax, where a branch is written as "{left,right}".
type tapTree struct {
	// leaf is the tapscript miniscript node of a leaf, or nil for a branch.
	leaf *node

	// left and right are the children of a branch, or nil for a leaf.
	left, right *tapTree
}

// forEachLeaf calls fn for every leaf of the tree in left-to-right order,
// passing each leaf's depth (0 for a single-leaf tree).
func (t *tapTree) forEachLeaf(depth int, fn func(leaf *node,
	depth int) error) error {

	if t.leaf != nil {
		return fn(t.leaf, depth)
	}
	if err := t.left.forEachLeaf(depth+1, fn); err != nil {
		return err
	}
	return t.right.forEachLeaf(depth+1, fn)
}

// Descriptor is a parsed output descriptor. Construct it with NewDescriptor;
// its zero value is not usable. Its methods may be called concurrently.
type Descriptor struct {
	// body is the descriptor string without the checksum suffix.
	body string

	// root is the parsed descriptor tree.
	root *node

	// keys holds every key in the descriptor, in the order they appear.
	keys []*descKey

	// keyByRaw maps a key's canonical string to its parsed form, for key
	// substitution during derivation.
	keyByRaw map[string]*descKey

	// multipath is the number of single-path sub-descriptors, i.e. the
	// multipath length (1 if the descriptor has no multipath element).
	multipath int
}

// NewDescriptor parses an output descriptor, verifies its checksum if present,
// and checks grammar, key forms and miniscript sanity. Key derivation and curve
// point validation are deferred until concrete scripts or addresses are needed.
func NewDescriptor(descriptor string) (*Descriptor, error) {
	body, err := stripChecksum(descriptor)
	if err != nil {
		return nil, err
	}

	var keys []*descKey
	root, err := parseNode(body, posTop, &keys)
	if err != nil {
		return nil, err
	}

	// Determine the multipath length: every multipath element in the
	// descriptor must have the same length.
	multipath := 1
	for _, k := range keys {
		l := k.multipathLen()
		if l == 1 {
			continue
		}
		if multipath != 1 && multipath != l {
			return nil, fmt.Errorf("descriptor contains multipath "+
				"elements of differing lengths (%d and %d)",
				multipath, l)
		}
		multipath = l
	}

	keyByRaw := make(map[string]*descKey, len(keys))
	for _, k := range keys {
		keyByRaw[k.raw] = k
	}

	return &Descriptor{
		body:      body,
		root:      root,
		keys:      keys,
		keyByRaw:  keyByRaw,
		multipath: multipath,
	}, nil
}

// stripChecksum splits off and verifies an optional "#checksum" suffix and
// returns the descriptor body without it. The body is always validated against
// the BIP380 descriptor character set, even when no checksum is supplied, so
// that every accepted descriptor is round-trippable through String().
func stripChecksum(descriptor string) (string, error) {
	body, got, hasChecksum := strings.Cut(descriptor, "#")

	// descriptorChecksum returns the empty string if the body contains a
	// character outside the allowed set, which makes the descriptor invalid
	// whether or not a checksum was supplied.
	want := descriptorChecksum(body)
	if want == "" {
		return "", fmt.Errorf("descriptor contains invalid characters")
	}
	if hasChecksum && got != want {
		return "", fmt.Errorf("invalid descriptor checksum: got %q, "+
			"expected %q", got, want)
	}

	return body, nil
}

// parseNode parses a descriptor sub-expression appearing in the given position,
// which decides the key serializations that are valid in it and the miniscript
// context a bare miniscript is compiled in. Parsed keys are appended to keys in
// order.
//
// The recursion of this function, and of everything that walks the parsed tree
// afterwards, is bounded, which matters because Go turns a deep enough
// recursion into a fatal stack overflow that recover() cannot catch, killing
// the process rather than the request:
//
//   - parseNode recurses only into an sh() or wsh() inner, and the position rules
//     allow sh() only at the top level and wsh() only at the top level or inside
//     an sh(), so the deepest chain is sh(wsh(...)): three levels. Any new
//     expression kind that may nest has to be checked against this.
//   - parseTapTree, and the tapNode, forEachLeaf, collectLeafPlans and
//     liftTapTree walks of the parsed tree, are bounded by the maximum tap tree
//     depth of 128.
//   - a miniscript inner is bounded by the miniscript package's own nesting
//     limit, which it enforces during parsing.
func parseNode(s string, pos scriptPos, keys *[]*descKey) (*node, error) {
	name, inner, ok := splitFunc(s)
	if !ok {
		// A fragment without a "name(...)" form is a bare miniscript
		// primitive such as "0" or "1"; let the miniscript parser
		// validate it in the current context.
		return parseMs(s, pos, keys)
	}

	// An expression that is a descriptor script expression rather than a
	// miniscript fragment is only valid in the positions its BIP allows,
	// which rejects nestings such as sh(sh(...)) or sh(tr(...)) here
	// instead of letting them fail at address derivation time.
	if kind := nodeKind(name); kind.isDescriptorExpr() &&
		!kind.allowedIn(pos) {

		return nil, fmt.Errorf("%s() cannot be used %s", kind, pos)
	}

	switch nodeKind(name) {
	case nodePk, nodePkh:
		key, err := parseDescKey(inner, pos.keyForm())
		if err != nil {
			return nil, err
		}
		*keys = append(*keys, key)
		return &node{
			kind: nodeKind(name),
			pos:  pos,
			keys: []*descKey{key},
		}, nil

	case nodeWpkh:
		// A P2WPKH commits to the key hash of a compressed key, in the
		// bare as well as in the P2SH-wrapped form (BIP382).
		key, err := parseDescKey(inner, keyFormCompressed)
		if err != nil {
			return nil, err
		}
		*keys = append(*keys, key)
		return &node{
			kind: nodeWpkh,
			pos:  pos,
			keys: []*descKey{key},
		}, nil

	case nodeSh:
		// The inner script of a P2SH is a legacy script, which for the
		// purpose of script generation uses the same fragment encodings
		// as P2WSH, but takes uncompressed keys in its key expressions
		// and is bound by the resource limits of a redeem script.
		sub, err := parseNode(inner, posSh, keys)
		if err != nil {
			return nil, err
		}
		if err := checkRedeemScript(sub); err != nil {
			return nil, err
		}
		return &node{kind: nodeSh, pos: pos, sub: sub}, nil

	case nodeWsh:
		sub, err := parseNode(inner, posWsh, keys)
		if err != nil {
			return nil, err
		}
		return &node{kind: nodeWsh, pos: pos, sub: sub}, nil

	case nodeTr:
		return parseTr(inner, keys)

	case nodeMulti, nodeSortedMulti:
		return parseMulti(nodeKind(name), inner, pos, keys)

	default:
		// Anything else is a miniscript expression compiled in the
		// current context.
		return parseMs(s, pos, keys)
	}
}

// checkRedeemScript returns an error if the given inner node cannot be used as
// the redeem script of a P2SH output.
//
// The redeem script is pushed as a single data element in the spending
// scriptSig, so it must not exceed the consensus limit on the size of a script
// element (520 bytes): coins sent to a P2SH address whose redeem script is
// larger are unspendable. The scriptSig also holds the satisfaction, and a
// scriptSig over the standardness limit (1650 bytes) is not relayed.
//
// A miniscript inner is compiled in the Legacy context, which enforces both
// limits itself, so only the nodes that build their script directly are checked
// here.
func checkRedeemScript(sub *node) error {
	switch sub.kind {
	case nodePk, nodePkh, nodeMulti, nodeSortedMulti:

	default:
		// A P2SH-wrapped segwit output commits to a fixed-size witness
		// program, which is far below either limit, and a miniscript
		// inner is compiled in the Legacy context, which enforces both
		// limits itself.
		return nil
	}

	scriptSize, satSize, _, err := satInfo(sub)
	if err != nil {
		return err
	}

	if scriptSize > maxRedeemScriptSize {
		return fmt.Errorf("the redeem script of the sh() descriptor is "+
			"%d bytes, which is larger than the maximum redeem "+
			"script size of %d", scriptSize, maxRedeemScriptSize)
	}

	// The scriptSig holds the satisfaction and the push of the redeem
	// script.
	scriptSig := satSize + pushOpcodeSize(scriptSize) + scriptSize
	if scriptSig > maxScriptSigSize {
		return fmt.Errorf("spending the sh() descriptor requires a "+
			"scriptSig of %d bytes, which is larger than the "+
			"maximum scriptSig size of %d", scriptSig,
			maxScriptSigSize)
	}

	return nil
}

// parseTr parses the inner arguments of a tr() descriptor: an internal key and
// an optional taproot script tree.
func parseTr(inner string, keys *[]*descKey) (*node, error) {
	args := splitArgs(inner)
	internal, err := parseDescKey(args[0], keyFormXOnly)
	if err != nil {
		return nil, err
	}
	*keys = append(*keys, internal)

	n := &node{kind: nodeTr, pos: posTop, keys: []*descKey{internal}}
	switch len(args) {
	case 1:

	case 2:
		tree, err := parseTapTree(args[1], 0, keys)
		if err != nil {
			return nil, err
		}
		n.tapTree = tree

	default:
		return nil, fmt.Errorf("tr() takes at most two arguments")
	}

	return n, nil
}

// parseTapTree parses a taproot script tree: either a "{left,right}" branch or
// a single tapscript leaf. A leaf is always a miniscript in the P2TR context,
// using x-only keys. depth is the level the tree starts at, i.e. the length of
// the merkle path a leaf at this position needs.
func parseTapTree(s string, depth int, keys *[]*descKey) (*tapTree, error) {
	if !strings.HasPrefix(s, "{") {
		leaf, err := parseMs(s, posTr, keys)
		if err != nil {
			return nil, err
		}
		return &tapTree{leaf: leaf}, nil
	}

	// Every level of the tree adds one 32-byte hash to the merkle path in
	// the control block of the leaves below it, and BIP341 allows at most
	// 128 of them, so the leaves of a deeper tree could never be spent.
	// Bounding the depth also bounds the parsing cost and the recursion of
	// every later walk of the tree (address derivation, planning, lifting).
	if depth >= maxTapTreeDepth {
		return nil, fmt.Errorf("the taproot script tree is deeper than "+
			"%d levels, which is more than the merkle path of a "+
			"control block can hold", maxTapTreeDepth)
	}

	if !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("malformed taproot tree branch %q", s)
	}
	leftStr, rightStr, err := splitTapBranch(s[1 : len(s)-1])
	if err != nil {
		return nil, err
	}

	left, err := parseTapTree(leftStr, depth+1, keys)
	if err != nil {
		return nil, err
	}
	right, err := parseTapTree(rightStr, depth+1, keys)
	if err != nil {
		return nil, err
	}

	return &tapTree{left: left, right: right}, nil
}

// splitTapBranch splits the content of a "{left,right}" taproot tree branch
// into its two children at the top-level comma.
//
// It stops at the first top-level comma instead of scanning the rest of the
// subtree for further arguments. A branch has exactly two children, so anything
// after that comma belongs to the right child, which is validated when it is
// parsed - except when the right child is a leaf, where a second top-level
// comma is checked for directly. This avoids repeatedly scanning the remaining
// right subtree of a right-skewed tree. Left subtrees still need scanning to
// locate the separator; the depth limit bounds how often they are revisited.
func splitTapBranch(s string) (string, string, error) {
	twoChildren := fmt.Errorf("taproot tree branch must have exactly two " +
		"children")

	comma := topLevelComma(s)
	if comma < 0 {
		return "", "", twoChildren
	}

	left, right := s[:comma], s[comma+1:]
	if !strings.HasPrefix(right, "{") && topLevelComma(right) >= 0 {
		return "", "", twoChildren
	}

	return left, right, nil
}

// topLevelComma returns the index of the first comma at the top nesting level
// of s, respecting (), {}, [] and <> grouping, or -1 if there is none.
func topLevelComma(s string) int {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(', '{', '[', '<':
			depth++

		case ')', '}', ']', '>':
			depth--

		case ',':
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// parseMulti parses the arguments of a multi()/sortedmulti() descriptor.
func parseMulti(kind nodeKind, inner string, pos scriptPos,
	keys *[]*descKey) (*node, error) {

	args := splitArgs(inner)
	if len(args) < 2 {
		return nil, fmt.Errorf("%s requires a threshold and at least "+
			"one key", kind)
	}

	// The threshold is a plain decimal number, which is what ParseUint
	// accepts and what the miniscript parser uses as well. strconv.Atoi
	// would also take a leading plus or minus sign, so multi(+1,KEY) used
	// to parse and round-trip with a valid checksum for a spelling that
	// Bitcoin Core rejects. The 16-bit size is far above the number of
	// keys a multisig may have, which the range check below enforces, and
	// keeps the conversion exact on every platform.
	k64, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid %s threshold %q: %w", kind,
			args[0], err)
	}

	k := int(k64)
	n := &node{kind: kind, pos: pos, thresh: k}
	for _, a := range args[1:] {
		key, err := parseDescKey(a, pos.keyForm())
		if err != nil {
			return nil, err
		}
		n.keys = append(n.keys, key)
		*keys = append(*keys, key)
	}

	// OP_CHECKMULTISIG fails at spend time for more than 20 keys, which is
	// a consensus rule, so any output whose script has more of them is
	// permanently unspendable.
	if len(n.keys) > maxMultisigKeys {
		return nil, fmt.Errorf("%s has %d keys, which is more than the "+
			"%d keys OP_CHECKMULTISIG accepts", kind, len(n.keys),
			maxMultisigKeys)
	}

	// A bare multisig is only standard with up to three keys (BIP383), so a
	// larger one is not relayed. Inside an sh() the redeem script size
	// limits the key count instead, and a wsh() takes the full 20.
	if pos == posTop && len(n.keys) > maxBareMultisigKeys {
		return nil, fmt.Errorf("a bare %s has %d keys, which is more "+
			"than the %d keys a bare multisig may have to be "+
			"standard", kind, len(n.keys), maxBareMultisigKeys)
	}
	if k < 1 || k > len(n.keys) {
		return nil, fmt.Errorf("%s threshold %d out of range for %d "+
			"keys", kind, k, len(n.keys))
	}

	return n, nil
}

// parseMs parses a bare miniscript expression appearing in the given position,
// collecting its keys.
//
// Miniscript is only defined for the wsh() and tr() contexts (BIP379), so its
// keys are always compressed, or x-only in a tapscript leaf: the pre-segwit
// positions that take uncompressed keys are the key expressions of
// pk/pkh/multi/sortedmulti, which are parsed as their own descriptor nodes.
func parseMs(s string, pos scriptPos, keys *[]*descKey) (*node, error) {
	ctx := miniscript.P2WSH
	keyFrm := keyFormCompressed
	switch pos {
	case posTop:
		// BIP379 defines miniscript expressions for the wsh() and tr()
		// contexts, to which this package adds sh() (see the README).
		// A bare one is of no use even where it is accepted: an output
		// script that is not one of the standard templates cannot be
		// paid to by a standard transaction in the first place. Core
		// rejects it as well (descriptor.cpp:2682 in Core
		// c4fbd3c7211).
		return nil, fmt.Errorf("a miniscript expression cannot be "+
			"used %s, only inside a wsh(), tr() or sh() one", pos)

	case posTr:
		ctx, keyFrm = miniscript.P2TR, keyFormXOnly

	case posSh:
		// A miniscript inside an sh() is a redeem script, whose
		// resource limits are those of the Legacy context.
		ctx = miniscript.Legacy
	}

	ast, err := miniscript.Parse(s, ctx)
	if err != nil {
		return nil, err
	}
	for _, keyStr := range ast.Keys() {
		key, err := parseDescKey(keyStr, keyFrm)
		if err != nil {
			return nil, err
		}
		*keys = append(*keys, key)
	}

	return &node{
		kind:   nodeMs,
		pos:    pos,
		msExpr: s,
		msCtx:  ctx,
		msAST:  ast,
	}, nil
}

// splitFunc splits a "name(inner)" expression into its name and inner content.
func splitFunc(s string) (name, inner string, ok bool) {
	open := strings.IndexByte(s, '(')
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	return s[:open], s[open+1 : len(s)-1], true
}

// splitArgs splits a comma-separated argument list at the top nesting level,
// respecting (), {}, [] and <> grouping.
func splitArgs(s string) []string {
	var (
		args  []string
		depth int
		start int
	)
	for i, ch := range s {
		switch ch {
		case '(', '{', '[', '<':
			depth++

		case ')', '}', ']', '>':
			depth--

		case ',':
			if depth == 0 {
				args = append(args, s[start:i])
				start = i + 1
			}
		}
	}
	return append(args, s[start:])
}

// String returns the complete string representation of the descriptor,
// including the checksum.
func (d *Descriptor) String() string {
	return d.body + "#" + descriptorChecksum(d.body)
}

// MultipathLen returns the number of single-path sub-descriptors (1 if there
// is no multipath element). Each multipath element must have that many entries.
func (d *Descriptor) MultipathLen() int {
	return d.multipath
}

// Keys returns all keys present in the descriptor, in the order they appear in
// the descriptor string.
func (d *Descriptor) Keys() []string {
	result := make([]string, len(d.keys))
	for i, k := range d.keys {
		result[i] = k.raw
	}
	return result
}

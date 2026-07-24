package descriptors

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/descriptors/miniscript"
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

// node is a parsed descriptor tree node.
type node struct {
	kind nodeKind

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
	// and the context (P2WSH or P2TR) it is compiled in.
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
	// DescTypeBare is a bare descriptor (containing a native P2PK).
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
		// A top-level pk() or bare miniscript.
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

// Descriptor is a parsed output descriptor.
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

// NewDescriptor parses the given descriptor string and returns a new Descriptor
// instance.
func NewDescriptor(descriptor string) (*Descriptor, error) {
	body, err := stripChecksum(descriptor)
	if err != nil {
		return nil, err
	}

	var keys []*descKey
	root, err := parseNode(body, miniscript.P2WSH, &keys)
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
	body := descriptor
	got, hasChecksum := "", false
	if idx := strings.IndexByte(descriptor, '#'); idx >= 0 {
		body = descriptor[:idx]
		got = descriptor[idx+1:]
		hasChecksum = true
	}

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

// parseNode parses a descriptor sub-expression. msCtx is the miniscript context
// to use if the expression turns out to be a bare miniscript (the inner of a
// wsh/sh, or a taproot leaf). Parsed keys are appended to keys in order.
func parseNode(s string, msCtx miniscript.Context, keys *[]*descKey) (*node,
	error) {

	name, inner, ok := splitFunc(s)
	if !ok {
		// A fragment without a "name(...)" form is a bare miniscript
		// primitive such as "0" or "1"; let the miniscript parser
		// validate it in the current context.
		return parseMs(s, msCtx, keys)
	}

	switch nodeKind(name) {
	case nodePk, nodePkh, nodeWpkh:
		key, err := parseDescKey(inner)
		if err != nil {
			return nil, err
		}
		*keys = append(*keys, key)
		return &node{kind: nodeKind(name), keys: []*descKey{key}}, nil

	case nodeSh:
		// The inner script of a P2SH is a legacy script; for the
		// purpose of script generation it uses the same fragment
		// encodings and compressed keys as P2WSH.
		sub, err := parseNode(inner, miniscript.P2WSH, keys)
		if err != nil {
			return nil, err
		}
		return &node{kind: nodeSh, sub: sub}, nil

	case nodeWsh:
		sub, err := parseNode(inner, miniscript.P2WSH, keys)
		if err != nil {
			return nil, err
		}
		return &node{kind: nodeWsh, sub: sub}, nil

	case nodeTr:
		return parseTr(inner, keys)

	case nodeMulti, nodeSortedMulti:
		return parseMulti(nodeKind(name), inner, keys)

	default:
		// Anything else is a miniscript expression compiled in the
		// current context.
		return parseMs(s, msCtx, keys)
	}
}

// parseTr parses the inner arguments of a tr() descriptor: an internal key and
// an optional taproot script tree.
func parseTr(inner string, keys *[]*descKey) (*node, error) {
	args := splitArgs(inner)
	internal, err := parseDescKey(args[0])
	if err != nil {
		return nil, err
	}
	*keys = append(*keys, internal)

	n := &node{kind: nodeTr, keys: []*descKey{internal}}
	switch len(args) {
	case 1:

	case 2:
		tree, err := parseTapTree(args[1], keys)
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
// using x-only keys.
func parseTapTree(s string, keys *[]*descKey) (*tapTree, error) {
	if !strings.HasPrefix(s, "{") {
		leaf, err := parseMs(s, miniscript.P2TR, keys)
		if err != nil {
			return nil, err
		}
		return &tapTree{leaf: leaf}, nil
	}

	if !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("malformed taproot tree branch %q", s)
	}
	children := splitArgs(s[1 : len(s)-1])
	if len(children) != 2 {
		return nil, fmt.Errorf("taproot tree branch must have exactly "+
			"two children, got %d", len(children))
	}

	left, err := parseTapTree(children[0], keys)
	if err != nil {
		return nil, err
	}
	right, err := parseTapTree(children[1], keys)
	if err != nil {
		return nil, err
	}

	return &tapTree{left: left, right: right}, nil
}

// parseMulti parses the arguments of a multi()/sortedmulti() descriptor.
func parseMulti(kind nodeKind, inner string, keys *[]*descKey) (*node, error) {
	args := splitArgs(inner)
	if len(args) < 2 {
		return nil, fmt.Errorf("%s requires a threshold and at least "+
			"one key", kind)
	}
	k, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid %s threshold %q: %w", kind,
			args[0], err)
	}
	n := &node{kind: kind, thresh: k}
	for _, a := range args[1:] {
		key, err := parseDescKey(a)
		if err != nil {
			return nil, err
		}
		n.keys = append(n.keys, key)
		*keys = append(*keys, key)
	}
	if k < 1 || k > len(n.keys) {
		return nil, fmt.Errorf("%s threshold %d out of range for %d "+
			"keys", kind, k, len(n.keys))
	}

	return n, nil
}

// parseMs parses a bare miniscript expression in the given context, collecting
// its keys.
func parseMs(s string, ctx miniscript.Context, keys *[]*descKey) (*node,
	error) {

	ast, err := miniscript.Parse(s, ctx)
	if err != nil {
		return nil, err
	}
	for _, keyStr := range ast.Keys() {
		key, err := parseDescKey(keyStr)
		if err != nil {
			return nil, err
		}
		*keys = append(*keys, key)
	}

	return &node{kind: nodeMs, msExpr: s, msCtx: ctx, msAST: ast}, nil
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

// MultipathLen returns the number of multipath elements in the descriptor (1 if
// the descriptor has no multipath element).
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

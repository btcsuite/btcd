package descriptors

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/descriptors/miniscript"
)

// SemanticPolicyType identifies the kind of a semantic policy node.
type SemanticPolicyType string

const (
	// SemanticPolicyTypeUnsatisfiable is a policy that always fails.
	SemanticPolicyTypeUnsatisfiable SemanticPolicyType = "unsatisfiable"

	// SemanticPolicyTypeTrivial is a policy that always succeeds.
	SemanticPolicyTypeTrivial SemanticPolicyType = "trivial"

	// SemanticPolicyTypeKey requires a signature matching a public key.
	SemanticPolicyTypeKey SemanticPolicyType = "key"

	// SemanticPolicyTypeAfter is an absolute locktime constraint.
	SemanticPolicyTypeAfter SemanticPolicyType = "after"

	// SemanticPolicyTypeOlder is a relative locktime constraint.
	SemanticPolicyTypeOlder SemanticPolicyType = "older"

	// SemanticPolicyTypeSha256 requires a SHA256 preimage.
	SemanticPolicyTypeSha256 SemanticPolicyType = "sha256"

	// SemanticPolicyTypeHash256 requires a double-SHA256 preimage.
	SemanticPolicyTypeHash256 SemanticPolicyType = "hash256"

	// SemanticPolicyTypeRipemd160 requires a RIPEMD160 preimage.
	SemanticPolicyTypeRipemd160 SemanticPolicyType = "ripemd160"

	// SemanticPolicyTypeHash160 requires a HASH160 preimage.
	SemanticPolicyTypeHash160 SemanticPolicyType = "hash160"

	// SemanticPolicyTypeThresh is a threshold combination of policies.
	SemanticPolicyTypeThresh SemanticPolicyType = "thresh"
)

// SemanticPolicy is an abstract policy corresponding to the semantics of a
// descriptor, allowing analysis such as filtering and normalization.
type SemanticPolicy struct {
	// Type is the kind of the semantic policy (always present).
	Type SemanticPolicyType `json:"type"`

	// Key is the public key string (present when Type is "key").
	Key *string `json:"key,omitempty"`

	// LockTime is the locktime value as a consensus number (present when
	// Type is "after" or "older").
	LockTime *uint32 `json:"lockTime,omitempty"`

	// Hash is the hex-encoded hash value (present for the hash types).
	Hash *string `json:"hash,omitempty"`

	// Threshold is the required threshold count (present when Type is
	// "thresh").
	Threshold *uint `json:"threshold,omitempty"`

	// Policies are the nested policies for threshold composition (present
	// when Type is "thresh").
	Policies []*SemanticPolicy `json:"policies,omitempty"`
}

// Lift converts this descriptor into its abstract semantic policy. It mirrors
// rust-miniscript's Descriptor::lift.
func (d *Descriptor) Lift() (*SemanticPolicy, error) {
	return d.liftNode(d.root)
}

// liftNode lifts a descriptor's top-level node, dispatching on the output type.
func (d *Descriptor) liftNode(n *node) (*SemanticPolicy, error) {
	switch n.kind {
	case nodeWpkh, nodePkh, nodePk:
		// A single-key output lifts to a key policy.
		return keyPolicy(n.keys[0].raw), nil

	case nodeWsh:
		return d.liftScript(n.sub)

	case nodeSh:
		return d.liftSh(n.sub)

	case nodeTr:
		internal := keyPolicy(n.keys[0].raw)

		// A key-path-only taproot output lifts to the internal key.
		if n.tapTree == nil {
			return internal, nil
		}

		// A taproot output with a script tree lifts to a k=1 threshold
		// (key path or script tree), without re-normalizing the top
		// level, matching rust-miniscript.
		tree, err := d.liftTapTree(n.tapTree)
		if err != nil {
			return nil, err
		}
		one := uint(1)
		return &SemanticPolicy{
			Type:      SemanticPolicyTypeThresh,
			Threshold: &one,
			Policies:  []*SemanticPolicy{internal, tree},
		}, nil

	default:
		// A bare miniscript output.
		return d.liftScript(n)
	}
}

// liftTapTree lifts a taproot script tree: every leaf is lifted and the leaves
// are combined under a k=1 threshold (a spend uses any one leaf), then
// normalized. It mirrors rust-miniscript's TapTree::lift.
func (d *Descriptor) liftTapTree(t *tapTree) (*SemanticPolicy, error) {
	var leaves []*miniscript.Policy
	err := t.forEachLeaf(0, func(leaf *node, _ int) error {
		policy, err := leaf.clonedMsAST().Lift()
		if err != nil {
			return err
		}
		leaves = append(leaves, policy)
		return nil
	})
	if err != nil {
		return nil, err
	}

	combined := (&miniscript.Policy{
		Type: miniscript.PolicyThresh,
		K:    1,
		Subs: leaves,
	}).Normalize()

	return fromMiniscriptPolicy(combined), nil
}

// liftSh lifts the inner node of a P2SH output.
func (d *Descriptor) liftSh(sub *node) (*SemanticPolicy, error) {
	switch sub.kind {
	case nodeWsh:
		return d.liftScript(sub.sub)

	case nodeWpkh:
		return keyPolicy(sub.keys[0].raw), nil

	default:
		return d.liftScript(sub)
	}
}

// liftScript lifts the inner script of a wsh/sh output, a taproot leaf, or a
// bare script.
func (d *Descriptor) liftScript(n *node) (*SemanticPolicy, error) {
	switch n.kind {
	case nodeMs:
		policy, err := n.clonedMsAST().Lift()
		if err != nil {
			return nil, err
		}
		return fromMiniscriptPolicy(policy), nil

	case nodeMulti, nodeSortedMulti:
		// A multisig lifts to a threshold of key policies. A single-key
		// multisig normalizes down to just that key.
		subs := make([]*SemanticPolicy, len(n.keys))
		for i, key := range n.keys {
			subs[i] = keyPolicy(key.raw)
		}
		if len(subs) == 1 {
			return subs[0], nil
		}
		thresh := uint(n.thresh)
		return &SemanticPolicy{
			Type:      SemanticPolicyTypeThresh,
			Threshold: &thresh,
			Policies:  subs,
		}, nil

	case nodePk, nodePkh:
		return keyPolicy(n.keys[0].raw), nil

	default:
		return nil, errUnsupportedInner
	}
}

// keyPolicy returns a key semantic policy for the given key string.
func keyPolicy(key string) *SemanticPolicy {
	k := key
	return &SemanticPolicy{Type: SemanticPolicyTypeKey, Key: &k}
}

// fromMiniscriptPolicy converts a miniscript semantic policy into the
// descriptor-level SemanticPolicy representation.
func fromMiniscriptPolicy(p *miniscript.Policy) *SemanticPolicy {
	switch p.Type {
	case miniscript.PolicyUnsatisfiable:
		return &SemanticPolicy{Type: SemanticPolicyTypeUnsatisfiable}

	case miniscript.PolicyTrivial:
		return &SemanticPolicy{Type: SemanticPolicyTypeTrivial}

	case miniscript.PolicyKey:
		return keyPolicy(p.Key)

	case miniscript.PolicyAfter:
		lt := p.Locktime
		return &SemanticPolicy{
			Type: SemanticPolicyTypeAfter, LockTime: &lt,
		}

	case miniscript.PolicyOlder:
		lt := p.Locktime
		return &SemanticPolicy{
			Type: SemanticPolicyTypeOlder, LockTime: &lt,
		}

	case miniscript.PolicySha256:
		return hashPolicy(SemanticPolicyTypeSha256, p.Hash)

	case miniscript.PolicyHash256:
		return hashPolicy(SemanticPolicyTypeHash256, p.Hash)

	case miniscript.PolicyRipemd160:
		return hashPolicy(SemanticPolicyTypeRipemd160, p.Hash)

	case miniscript.PolicyHash160:
		return hashPolicy(SemanticPolicyTypeHash160, p.Hash)

	case miniscript.PolicyThresh:
		thresh := uint(p.K)
		subs := make([]*SemanticPolicy, len(p.Subs))
		for i, sub := range p.Subs {
			subs[i] = fromMiniscriptPolicy(sub)
		}
		return &SemanticPolicy{
			Type:      SemanticPolicyTypeThresh,
			Threshold: &thresh,
			Policies:  subs,
		}

	default:
		return nil
	}
}

// hashPolicy returns a hash semantic policy of the given type with a
// hex-encoded hash value.
func hashPolicy(t SemanticPolicyType, hash []byte) *SemanticPolicy {
	h := hex.EncodeToString(hash)
	return &SemanticPolicy{Type: t, Hash: &h}
}

// String returns the canonical rust-miniscript display form of the policy: keys
// as pk(...), locktimes and hashes as their named forms, k=n thresholds as
// and(...), k=1 thresholds as or(...), and other thresholds as thresh(k, ...).
func (p *SemanticPolicy) String() string {
	switch p.Type {
	case SemanticPolicyTypeUnsatisfiable:
		return "UNSATISFIABLE"

	case SemanticPolicyTypeTrivial:
		return "TRIVIAL"

	case SemanticPolicyTypeKey:
		return fmt.Sprintf("pk(%s)", *p.Key)

	case SemanticPolicyTypeAfter:
		return fmt.Sprintf("after(%d)", *p.LockTime)

	case SemanticPolicyTypeOlder:
		return fmt.Sprintf("older(%d)", *p.LockTime)

	case SemanticPolicyTypeSha256, SemanticPolicyTypeHash256,
		SemanticPolicyTypeRipemd160, SemanticPolicyTypeHash160:

		return fmt.Sprintf("%s(%s)", p.Type, *p.Hash)

	case SemanticPolicyTypeThresh:
		subs := make([]string, len(p.Policies))
		for i, sub := range p.Policies {
			subs[i] = sub.String()
		}
		k, n := int(*p.Threshold), len(p.Policies)
		switch {
		case k == n:
			return fmt.Sprintf("and(%s)", strings.Join(subs, ","))

		case k == 1:
			return fmt.Sprintf("or(%s)", strings.Join(subs, ","))

		default:
			return fmt.Sprintf("thresh(%s,%s)",
				strconv.Itoa(k), strings.Join(subs, ","))
		}

	default:
		return "<unknown>"
	}
}

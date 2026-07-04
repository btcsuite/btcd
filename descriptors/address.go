package descriptors

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/btcsuite/btcd/txscript/v2"
)

// AddressAt derives and returns the address at the given multipath and
// derivation index for the given network.
func (d *Descriptor) AddressAt(params *chaincfg.Params, multipathIndex,
	derivationIndex uint32) (string, error) {

	if uint64(multipathIndex) >= uint64(d.multipath) {
		return "", fmt.Errorf("multipath index out of bounds")
	}

	addr, err := d.address(
		d.root, params, multipathIndex, derivationIndex,
	)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

// address builds the address for the given top-level node.
func (d *Descriptor) address(n *node, params *chaincfg.Params, mp,
	idx uint32) (address.Address, error) {

	switch n.kind {
	case nodePkh:
		pk, err := n.keys[0].derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		return address.NewAddressPubKeyHash(
			address.Hash160(pk), params,
		)

	case nodeWpkh:
		pk, err := n.keys[0].derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		return address.NewAddressWitnessPubKeyHash(
			address.Hash160(pk), params,
		)

	case nodeWsh:
		script, err := d.innerScript(n.sub, mp, idx)
		if err != nil {
			return nil, err
		}
		return address.NewAddressWitnessScriptHash(
			chainhash.HashB(script), params,
		)

	case nodeSh:
		redeem, err := d.redeemScript(n.sub, mp, idx)
		if err != nil {
			return nil, err
		}
		return address.NewAddressScriptHash(redeem, params)

	case nodeTr:
		outputKey, err := d.taprootOutputKey(n, mp, idx)
		if err != nil {
			return nil, err
		}
		return address.NewAddressTaproot(
			schnorr.SerializePubKey(outputKey), params,
		)

	default:
		return nil, fmt.Errorf("descriptor of type %q has no address",
			d.DescType())
	}
}

// redeemScript returns the script that a P2SH output commits to for the given
// inner node.
func (d *Descriptor) redeemScript(n *node, mp, idx uint32) ([]byte, error) {
	switch n.kind {
	case nodeWpkh:
		pk, err := n.keys[0].derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		return witnessV0Script(address.Hash160(pk))

	case nodeWsh:
		script, err := d.innerScript(n.sub, mp, idx)
		if err != nil {
			return nil, err
		}
		return witnessV0Script(chainhash.HashB(script))

	default:
		return d.innerScript(n, mp, idx)
	}
}

// innerScript builds the script of a wsh/sh content node (pk, pkh, multi,
// sortedmulti or a miniscript expression).
func (d *Descriptor) innerScript(n *node, mp, idx uint32) ([]byte, error) {
	b := txscript.NewScriptBuilder()

	switch n.kind {
	case nodePk:
		pk, err := n.keys[0].derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		b.AddData(pk)
		b.AddOp(txscript.OP_CHECKSIG)
		return b.Script()

	case nodePkh:
		pk, err := n.keys[0].derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		b.AddOp(txscript.OP_DUP)
		b.AddOp(txscript.OP_HASH160)
		b.AddData(address.Hash160(pk))
		b.AddOp(txscript.OP_EQUALVERIFY)
		b.AddOp(txscript.OP_CHECKSIG)
		return b.Script()

	case nodeMulti, nodeSortedMulti:
		return d.multiScript(n, mp, idx)

	case nodeMs:
		return d.miniscriptScript(n, mp, idx)

	default:
		return nil, fmt.Errorf("cannot build script for node %q",
			n.kind)
	}
}

// multiScript builds a bare CHECKMULTISIG script for a multi/sortedmulti node.
func (d *Descriptor) multiScript(n *node, mp, idx uint32) ([]byte, error) {
	pubKeys := make([][]byte, len(n.keys))
	for i, k := range n.keys {
		pk, err := k.derive(mp, idx, false)
		if err != nil {
			return nil, err
		}
		pubKeys[i] = pk
	}

	// sortedmulti sorts the keys lexicographically (BIP67) before building
	// the script.
	if n.kind == nodeSortedMulti {
		sort.Slice(pubKeys, func(i, j int) bool {
			return bytes.Compare(pubKeys[i], pubKeys[j]) < 0
		})
	}

	b := txscript.NewScriptBuilder()
	b.AddInt64(int64(n.thresh))
	for _, pk := range pubKeys {
		b.AddData(pk)
	}
	b.AddInt64(int64(len(pubKeys)))
	b.AddOp(txscript.OP_CHECKMULTISIG)
	return b.Script()
}

// miniscriptScript compiles a miniscript node into its script, substituting
// each key with its concrete derived public key.
func (d *Descriptor) miniscriptScript(n *node, mp, idx uint32) ([]byte, error) {
	ast := n.clonedMsAST()

	xOnly := n.msCtx == miniscript.P2TR
	err := ast.ApplyVars(func(id string) ([]byte, error) {
		k, ok := d.keyByRaw[id]
		if !ok {
			return nil, fmt.Errorf("unknown key %q", id)
		}
		return k.derive(mp, idx, xOnly)
	})
	if err != nil {
		return nil, err
	}

	return ast.Script()
}

// taprootOutputKey derives the taproot output key of a tr node: the internal
// key tweaked with the merkle root of its script tree (or without a tweak
// script for a key-path-only output).
func (d *Descriptor) taprootOutputKey(n *node, mp, idx uint32) (
	*btcec.PublicKey, error) {

	internal, err := n.keys[0].derivePub(mp, idx)
	if err != nil {
		return nil, err
	}

	if n.tapTree == nil {
		return txscript.ComputeTaprootKeyNoScript(internal), nil
	}

	root, err := d.tapNode(n.tapTree, mp, idx)
	if err != nil {
		return nil, err
	}
	rootHash := root.TapHash()

	return txscript.ComputeTaprootOutputKey(internal, rootHash[:]), nil
}

// tapNode builds the txscript taproot tree node for a descriptor tap tree,
// deriving each leaf's script at the given multipath and derivation index. Its
// TapHash is the merkle root committed to by the taproot output key.
func (d *Descriptor) tapNode(t *tapTree, mp, idx uint32) (txscript.TapNode,
	error) {

	if t.leaf != nil {
		leafScript, err := d.innerScript(t.leaf, mp, idx)
		if err != nil {
			return nil, err
		}
		return txscript.NewBaseTapLeaf(leafScript), nil
	}

	left, err := d.tapNode(t.left, mp, idx)
	if err != nil {
		return nil, err
	}
	right, err := d.tapNode(t.right, mp, idx)
	if err != nil {
		return nil, err
	}

	return txscript.NewTapBranch(left, right), nil
}

// witnessV0Script builds a version-0 witness program script (OP_0 <program>),
// used both as the scriptPubKey of a P2WPKH/P2WSH output and as the redeem
// script of a P2SH-wrapped one.
func witnessV0Script(program []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(program).
		Script()
}

// ScriptCodeAt derives and returns the script code (the raw compiled Bitcoin
// script used for sighash computation) at the given multipath and derivation
// index.
func (d *Descriptor) ScriptCodeAt(multipathIndex, derivationIndex uint32) (
	[]byte, error) {

	if uint64(multipathIndex) >= uint64(d.multipath) {
		return nil, fmt.Errorf("multipath index out of bounds")
	}
	return d.scriptCode(d.root, multipathIndex, derivationIndex)
}

// scriptCode returns the script code for the given top-level node.
func (d *Descriptor) scriptCode(n *node, mp, idx uint32) ([]byte, error) {
	switch n.kind {
	case nodePkh, nodePk:
		return d.innerScript(n, mp, idx)

	case nodeWpkh:
		// The script code of a P2WPKH is the corresponding P2PKH
		// script.
		return d.innerScript(&node{
			kind: nodePkh,
			keys: []*descKey{n.keys[0]},
		}, mp, idx)

	case nodeWsh:
		return d.innerScript(n.sub, mp, idx)

	case nodeSh:
		if n.sub.kind == nodeWsh {
			return d.innerScript(n.sub.sub, mp, idx)
		}
		if n.sub.kind == nodeWpkh {
			return d.scriptCode(n.sub, mp, idx)
		}
		return d.innerScript(n.sub, mp, idx)

	default:
		return nil, fmt.Errorf("descriptor of type %q has no script "+
			"code", d.DescType())
	}
}

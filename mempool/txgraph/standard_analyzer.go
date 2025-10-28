// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txgraph

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const (
	// TRUCVersion is the transaction version that signals TRUC
	// (Topologically Restricted Until Confirmation) policy as defined in
	// BIP 431. Transactions with nVersion=3 are subject to special topology
	// and size restrictions to prevent pinning attacks.
	TRUCVersion = 3

	// MaxTRUCParentSize is the maximum sigop-adjusted virtual size in vB
	// for a TRUC transaction with no unconfirmed ancestors. This limit
	// prevents excessively large parent transactions while allowing space
	// for payments, HTLCs, and other contracting protocol uses.
	// Per BIP 431 Rule 4.
	MaxTRUCParentSize = 10000

	// MaxTRUCChildSize is the maximum sigop-adjusted virtual size in vB for
	// a TRUC transaction that has an unconfirmed TRUC ancestor. This
	// smaller limit for child transactions ensures fee-bumping remains
	// economical while allowing sufficient space for fee-bumping inputs.
	// Per BIP 431 Rule 5.
	MaxTRUCChildSize = 1000

)

// StandardPackageAnalyzer implements the PackageAnalyzer interface with
// Bitcoin Core-compatible package validation rules. This analyzer enforces
// BIP 431 TRUC topology restrictions and will support ephemeral anchor
// validation in the future.
type StandardPackageAnalyzer struct {
	// Currently stateless, but struct allows future extension with
	// configuration if needed.
}

// NewStandardPackageAnalyzer creates a new standard package analyzer that
// enforces BIP 431 TRUC rules and standard package policies.
func NewStandardPackageAnalyzer() *StandardPackageAnalyzer {
	return &StandardPackageAnalyzer{}
}

// IsTRUCTransaction checks if a transaction is version 3 and thus subject to
// TRUC topology restrictions per BIP 431.
//
// TRUC (Topologically Restricted Until Confirmation) transactions use
// nVersion=3 to signal that they should be subject to stricter topology limits
// (1-parent-1-child) to prevent transaction pinning attacks. This is critical
// for Lightning Network and other contracting protocols.
func (a *StandardPackageAnalyzer) IsTRUCTransaction(tx *wire.MsgTx) bool {
	return tx.Version == TRUCVersion
}

// IsZeroFee identifies transactions with zero fees, which require special
// handling in packages since they cannot be mined alone. This is critical for
// CPFP validation and ephemeral dust validation.
//
// TRUC allows zero-fee parent transactions when they are part of a package
// that meets the mempool's feerate requirements (BIP 431 Rule 6).
func (a *StandardPackageAnalyzer) IsZeroFee(desc *TxDesc) bool {
	return desc.Fee == 0
}

// HasEphemeralDust checks if a transaction contains ephemeral dust outputs
// that must be spent in the same package.
//
// TODO: Implement P2A (Pay-to-Anchor) detection for ephemeral anchors.
// Currently returns false as ephemeral anchors are not yet implemented.
func (a *StandardPackageAnalyzer) HasEphemeralDust(tx *wire.MsgTx) bool {
	// Placeholder: Will detect P2A outputs in future implementation.
	return false
}

// ValidateEphemeralPackage ensures all ephemeral dust outputs are spent
// within the package.
//
// TODO: Implement ephemeral dust validation.
// Currently returns true as ephemeral anchors are not yet implemented.
func (a *StandardPackageAnalyzer) ValidateEphemeralPackage(nodes []*TxGraphNode) bool {
	// Placeholder: Will validate ephemeral dust spending in future.
	return true
}

// ValidateTRUCPackage enforces BIP 431 topology restrictions on v3 packages.
// This includes:
//   - Rule 2: All-or-none TRUC (all unconfirmed ancestors/descendants must be v3)
//   - Rule 3: Topology limits (max 1 unconfirmed ancestor, max 1 unconfirmed descendant)
//   - Rule 4: Parent size limit (max 10,000 vB)
//   - Rule 5: Child size limit (max 1,000 vB if has unconfirmed ancestor)
//
// Note: Rule 1 (v3 signals replaceability) is handled in PolicyEnforcer.
// Note: Rule 6 (zero-fee allowed in packages) is handled in fee validation.
//
// Returns false if any TRUC rule is violated.
func (a *StandardPackageAnalyzer) ValidateTRUCPackage(pkg *TxPackage) bool {
	// BIP 431 Rule 3: "An unconfirmed TRUC transaction cannot have more
	// than 1 unconfirmed ancestor." This means the maximum chain depth is 1
	// (parent→child only, no grandparent→parent→child). The MaxDepth
	// topology metric counts ancestor generations within the package.
	if pkg.Topology.MaxDepth > 1 {
		return false
	}

	// Validate per-node TRUC constraints.
	for _, node := range pkg.Transactions {
		if !a.IsTRUCTransaction(node.Tx.MsgTx()) {
			continue
		}

		// Enforce all-or-none requirement: mixing v2/v3 in unconfirmed chains
		// would allow bypassing TRUC topology limits through non-v3 children.
		for _, parent := range node.Parents {
			if !a.IsTRUCTransaction(parent.Tx.MsgTx()) {
				return false
			}
		}

		for _, child := range node.Children {
			if !a.IsTRUCTransaction(child.Tx.MsgTx()) {
				return false
			}
		}

		// 1-parent-1-child topology prevents pinning through either absolute
		// fees (multiple descendants) or eviction count (multiple ancestors).
		if len(node.Parents) > 1 {
			return false
		}

		if len(node.Children) > 1 {
			return false
		}

		// Size limits bound the economic cost of replacing TRUC transactions,
		// making fee-bumping reliable for contracting protocols.
		vsize := node.TxDesc.VirtualSize

		if len(node.Parents) == 0 {
			if vsize > MaxTRUCParentSize {
				return false
			}
		} else {
			if vsize > MaxTRUCChildSize {
				return false
			}
		}
	}

	return true
}

// AnalyzePackageType determines the most specific package type based on
// structure and properties. This classification determines which validation
// rules apply to the package.
//
// Package types are checked in priority order from most to least restrictive:
//  1. 1P1C (one-parent-one-child): Exactly 2 transactions with single parent-child relationship.
//  2. TRUC (BIP 431): All transactions are v3, enforcing topological restrictions.
//  3. Ephemeral: Any transaction creates ephemeral dust outputs.
//  4. Standard: Default classification for all other packages.
func (a *StandardPackageAnalyzer) AnalyzePackageType(nodes []*TxGraphNode) PackageType {
	// Check for 1-parent-1-child topology by verifying exactly two
	// transactions with a single parent-child relationship.
	if len(nodes) == 2 {
		parentCount := 0
		childCount := 0

		// Create a map for quick lookup of nodes in this package.
		nodeMap := make(map[chainhash.Hash]bool, len(nodes))
		for _, node := range nodes {
			nodeMap[node.TxHash] = true
		}

		for _, node := range nodes {
			hasParentInPkg := false
			hasChildInPkg := false

			for parentHash := range node.Parents {
				if nodeMap[parentHash] {
					hasParentInPkg = true
					break
				}
			}

			for childHash := range node.Children {
				if nodeMap[childHash] {
					hasChildInPkg = true
					break
				}
			}

			if !hasParentInPkg {
				parentCount++
			}
			if !hasChildInPkg {
				childCount++
			}
		}

		if parentCount == 1 && childCount == 1 {
			return PackageType1P1C
		}
	}

	// Check for TRUC: If ANY transaction is v3, classify as TRUC package.
	// Per BIP 431 Rule 2, TRUC rules apply to any package containing v3
	// transactions, even if the package also contains v2 transactions.
	// The all-or-none requirement is enforced during validation, not
	// classification. This ensures mixed v2/v3 packages are properly
	// identified and rejected during TRUC validation.
	hasV3 := false
	for _, node := range nodes {
		if a.IsTRUCTransaction(node.Tx.MsgTx()) {
			hasV3 = true
			break
		}
	}
	if hasV3 && len(nodes) > 0 {
		return PackageTypeTRUC
	}

	// Check for ephemeral dust. If any transaction creates
	// ephemeral outputs, the entire package is classified as
	// ephemeral and must meet dust spending requirements.
	// This uses the analyzer's own detection method rather than cached metadata.
	for _, node := range nodes {
		if a.HasEphemeralDust(node.Tx.MsgTx()) {
			return PackageTypeEphemeral
		}
	}

	// Default to standard package type for all other cases.
	return PackageTypeStandard
}

// Ensure StandardPackageAnalyzer implements PackageAnalyzer.
var _ PackageAnalyzer = (*StandardPackageAnalyzer)(nil)

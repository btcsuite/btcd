package txgraph

import (
	"github.com/btcsuite/btcd/wire"
)

// PackageAnalyzer defines the interface for analyzing transaction packages.
// This abstraction allows for different implementations of package validation
// rules without coupling the graph to specific protocol details. This design
// enables testing with mock analyzers and future protocol upgrades without
// modifying core graph logic.
type PackageAnalyzer interface {
	// IsTRUCTransaction checks if a transaction is version 3 and thus
	// subject to TRUC topology restrictions. This enables the graph to
	// identify which transactions need special validation and to enforce
	// v3-specific relay policies.
	IsTRUCTransaction(tx *wire.MsgTx) bool

	// HasEphemeralDust checks if a transaction contains dust outputs that
	// must be spent in the same package. This enables enforcement of the
	// ephemeral dust policy where such outputs cannot exist unspent.
	HasEphemeralDust(tx *wire.MsgTx) bool

	// IsZeroFee identifies transactions with zero fees, which require
	// special handling in packages since they cannot be mined alone. This
	// is critical for CPFP and ephemeral dust validation.
	IsZeroFee(desc *TxDesc) bool

	// ValidateTRUCPackage enforces BIP 431 topology restrictions on v3
	// packages. This includes single-parent rules, size limits, depth
	// constraints, and topology restrictions that prevent pinning attacks.
	ValidateTRUCPackage(pkg *TxPackage) bool

	// ValidateEphemeralPackage ensures all ephemeral dust outputs are
	// spent within the package. This prevents unspendable dust from
	// entering the UTXO set and enables zero-fee parent transactions.
	ValidateEphemeralPackage(nodes []*TxGraphNode) bool

	// AnalyzePackageType determines the most specific package type
	// (1P1C, TRUC, ephemeral, standard) based on structure and properties.
	// This classification determines which validation rules apply.
	AnalyzePackageType(nodes []*TxGraphNode) PackageType
}
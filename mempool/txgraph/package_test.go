package txgraph

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCreatePackage verifies manual package creation through the graph
// API, ensuring that directly constructed packages maintain proper
// topology analysis and fee calculations. This is critical for package
// relay because nodes must be able to construct 1P1C packages on demand
// for relay validation without full package identification.
func TestCreatePackage(t *testing.T) {
	g := New(DefaultConfig())

	// Create simple parent-child pair. The child has higher fee to
	// demonstrate CPFP (Child Pays For Parent) economics.
	parent, parentDesc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
	// Set higher fee to simulate CPFP scenario where the child
	// incentivizes mining of the parent.
	childDesc.Fee = 2000
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Retrieve the graph nodes to construct a package manually.
	parentNode, exists := g.GetNode(*parent.Hash())
	require.True(t, exists)
	childNode, exists := g.GetNode(*child.Hash())
	require.True(t, exists)

	// Create package and verify it succeeds for connected transactions.
	pkg, err := g.CreatePackage([]*TxGraphNode{parentNode, childNode})
	require.NoError(t, err)
	require.NotNil(t, pkg)

	// Verify the package correctly identifies as 1P1C with proper
	// topology properties for relay validation.
	require.NotEmpty(t, pkg.ID)
	require.Equal(t, PackageType1P1C, pkg.Type)
	require.Len(t, pkg.Transactions, 2)
	require.Equal(t, int64(3000), pkg.TotalFees)
	require.True(t, pkg.Topology.IsLinear)
	require.True(t, pkg.Topology.IsTree)
	require.Equal(t, 1, pkg.Topology.MaxDepth)

	// Verify both transactions are properly indexed in the package.
	require.NotNil(t, pkg.Transactions[*parent.Hash()])
	require.NotNil(t, pkg.Transactions[*child.Hash()])
}

// TestTRUCPackageIdentification verifies that v3 transaction packages
// are correctly identified as TRUC packages per BIP 431. This ensures
// the graph can distinguish TRUC packages from standard packages,
// enabling enforcement of v3 topology restrictions (single parent,
// single child) during package relay validation.
func TestTRUCPackageIdentification(t *testing.T) {
	// Use a mock analyzer to simulate TRUC transaction detection
	// without requiring full BIP 431 validation logic.
	mockAnalyzer := new(MockPackageAnalyzer)
	config := DefaultConfig()
	config.PackageAnalyzer = mockAnalyzer

	g := New(config)

	// Create parent with version 3 to indicate TRUC transaction.
	parentTx := wire.NewMsgTx(3)
	parentTx.AddTxOut(wire.NewTxOut(100000, nil))
	parent := btcutil.NewTx(parentTx)
	parentDesc := &TxDesc{
		TxHash:      *parent.Hash(),
		VirtualSize: int64(parent.MsgTx().SerializeSize()),
		Fee:         1000,
		FeePerKB:    10000,
	}

	// Create child also with version 3 to form valid TRUC package.
	childTx := wire.NewMsgTx(3)
	childTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{
		Hash:  *parent.Hash(),
		Index: 0,
	}, nil, nil))
	childTx.AddTxOut(wire.NewTxOut(90000, nil))
	child := btcutil.NewTx(childTx)
	childDesc := &TxDesc{
		TxHash:      *child.Hash(),
		VirtualSize: int64(child.MsgTx().SerializeSize()),
		Fee:         2000,
		FeePerKB:    20000,
	}

	// Configure mock to report transactions as TRUC (v3).
	mockAnalyzer.On("IsTRUCTransaction", mock.Anything).Return(true)

	require.NoError(t, g.AddTransaction(parent, parentDesc))
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Trigger package identification to classify the package type.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify a TRUC package was identified with correct topology
	// constraints (depth 1, width 1) per BIP 431 requirements.
	found := false
	for _, pkg := range packages {
		if pkg.Type == PackageTypeTRUC {
			found = true
			require.Len(t, pkg.Transactions, 2)
			// TRUC packages enforce strict 1P1C topology for
			// relay rules.
			require.Equal(t, 1, pkg.Topology.MaxDepth)
			require.Equal(t, 1, pkg.Topology.MaxWidth)
		}
	}
	require.True(t, found, "Should identify TRUC package")

	mockAnalyzer.AssertExpectations(t)
}

// TestEphemeralPackageIdentification verifies that zero-fee transactions
// with ephemeral dust outputs are correctly classified as ephemeral
// packages. This is essential for package relay because ephemeral dust
// allows zero-fee parents to be relayed as long as a child spends the
// dust output and pays sufficient fees for the entire package.
func TestEphemeralPackageIdentification(t *testing.T) {
	// Use mock analyzer to simulate ephemeral dust detection logic
	// without full policy validation.
	mockAnalyzer := new(MockPackageAnalyzer)
	config := DefaultConfig()
	config.PackageAnalyzer = mockAnalyzer

	g := New(config)

	// Create parent with zero-value output representing ephemeral
	// dust that must be spent immediately.
	parentTx := wire.NewMsgTx(wire.TxVersion)
	parentTx.AddTxOut(wire.NewTxOut(0, nil))
	parent := btcutil.NewTx(parentTx)
	parentDesc := &TxDesc{
		TxHash:      *parent.Hash(),
		VirtualSize: int64(parent.MsgTx().SerializeSize()),
		// Zero fee is allowed because child will pay for the
		// entire package.
		Fee:      0,
		FeePerKB: 0,
	}

	// Create child that spends the ephemeral dust output, paying
	// fees for both transactions in the package.
	childTx := wire.NewMsgTx(wire.TxVersion)
	childTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{
		Hash:  *parent.Hash(),
		Index: 0,
	}, nil, nil))
	childTx.AddTxOut(wire.NewTxOut(100000, nil))
	child := btcutil.NewTx(childTx)
	childDesc := &TxDesc{
		TxHash:      *child.Hash(),
		VirtualSize: int64(child.MsgTx().SerializeSize()),
		Fee:         1000,
		FeePerKB:    10000,
	}

	// Configure mock to identify this as an ephemeral dust package.
	mockAnalyzer.On("IsTRUCTransaction", mock.Anything).Return(false)
	mockAnalyzer.On("HasEphemeralDust", mock.Anything).Return(true)
	mockAnalyzer.On("IsZeroFee", mock.Anything).Return(true)

	require.NoError(t, g.AddTransaction(parent, parentDesc))
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Trigger package identification to detect ephemeral package.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify an ephemeral package was identified, enabling special
	// relay treatment for zero-fee parents with dust outputs.
	found := false
	for _, pkg := range packages {
		if pkg.Type == PackageTypeEphemeral {
			found = true
			require.Len(t, pkg.Transactions, 2)
		}
	}
	require.True(t, found, "Should identify ephemeral package")

	mockAnalyzer.AssertExpectations(t)
}

// TestStandardPackageIdentification verifies that multi-child packages
// are classified as standard packages rather than specialized types.
// This is important for package relay because standard packages have
// different validation rules than 1P1C packages and may not qualify
// for certain relay optimizations.
func TestStandardPackageIdentification(t *testing.T) {
	g := New(DefaultConfig())

	// Create parent with two outputs to enable multiple children,
	// which disqualifies this from being a 1P1C package.
	parent, parentDesc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	// Create two children spending different outputs to form a
	// non-linear topology.
	child1, child1Desc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(child1, child1Desc))

	child2, child2Desc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 1}}, 1)
	require.NoError(t, g.AddTransaction(child2, child2Desc))

	// Trigger package identification to classify the package type.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify standard package classification due to having two
	// children, which violates 1P1C constraints.
	found := false
	for _, pkg := range packages {
		if pkg.Type == PackageTypeStandard {
			found = true
			require.Len(t, pkg.Transactions, 3)
			require.Equal(t, 1, pkg.Topology.MaxDepth)
			require.Equal(t, 2, pkg.Topology.MaxWidth)
			require.False(t, pkg.Topology.IsLinear)
			require.True(t, pkg.Topology.IsTree)
		}
	}
	require.True(t, found, "Should identify standard package")
}

// TestPackageTopologyCalculation verifies that complex DAG structures
// are correctly analyzed for depth, width, and tree properties. This
// is critical for package relay because topology properties determine
// which relay policies apply (e.g., TRUC requires linear topology,
// standard packages may allow more complex structures).
func TestPackageTopologyCalculation(t *testing.T) {
	g := New(DefaultConfig())

	// Create a diamond-shaped DAG structure with convergent paths
	// to test non-tree topology detection:
	//      root
	//     /    \
	//   mid1   mid2
	//     \    /
	//      leaf

	root, rootDesc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(root, rootDesc))

	mid1, mid1Desc := createTestTx(
		[]wire.OutPoint{{Hash: *root.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(mid1, mid1Desc))

	mid2, mid2Desc := createTestTx(
		[]wire.OutPoint{{Hash: *root.Hash(), Index: 1}}, 1)
	require.NoError(t, g.AddTransaction(mid2, mid2Desc))

	// Create leaf that spends from both mid transactions, forming
	// a diamond pattern with reconvergent paths.
	leaf, leafDesc := createTestTx([]wire.OutPoint{
		{Hash: *mid1.Hash(), Index: 0},
		{Hash: *mid2.Hash(), Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(leaf, leafDesc))

	// Trigger package identification to compute topology metrics.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify the diamond structure is recognized as a single
	// package with non-tree topology due to convergence.
	require.Len(t, packages, 1)
	pkg := packages[0]

	require.Equal(t, 2, pkg.Topology.MaxDepth)
	require.Equal(t, 2, pkg.Topology.MaxWidth)
	require.False(t, pkg.Topology.IsLinear)
	// Diamond pattern violates tree property due to multiple
	// paths to leaf node.
	require.False(t, pkg.Topology.IsTree)
	require.Equal(t, 4, pkg.Topology.TotalNodes)
}

// TestPackageWithDisconnectedNodes verifies that package creation
// fails when given unrelated transactions with no spending
// relationship. This validates the package invariant that all
// transactions must form a connected subgraph, which is required for
// package relay because relay policies assume topological ordering.
func TestPackageWithDisconnectedNodes(t *testing.T) {
	g := New(DefaultConfig())

	// Create two transactions with no spending relationship to
	// test disconnected graph detection.
	tx1, desc1 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx1, desc1))

	tx2, desc2 := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx2, desc2))

	// Retrieve both nodes to attempt invalid package construction.
	node1, _ := g.GetNode(*tx1.Hash())
	node2, _ := g.GetNode(*tx2.Hash())

	// Verify that attempting to create a package from disconnected
	// transactions returns an error, preventing invalid packages.
	_, err := g.CreatePackage([]*TxGraphNode{node1, node2})
	require.Error(t, err)
	require.Contains(t, err.Error(), "disconnected")
}

// TestPackageFeeCalculation verifies that package-level fee rates are
// correctly computed as the aggregate fee divided by aggregate size.
// This is essential for package relay because miners prioritize
// packages by aggregate fee rate, and CPFP scenarios depend on
// accurate package-level fee calculations to incentivize mining.
func TestPackageFeeCalculation(t *testing.T) {
	g := New(DefaultConfig())

	// Create parent with low individual fee rate to simulate
	// transactions that benefit from CPFP.
	parent, parentDesc := createTestTx(nil, 1)
	parentDesc.Fee = 100
	parentDesc.VirtualSize = 100
	parentDesc.FeePerKB = 1000
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	// Create child with high fee to boost the package fee rate,
	// demonstrating CPFP economics.
	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
	childDesc.Fee = 900
	childDesc.VirtualSize = 100
	childDesc.FeePerKB = 9000
	require.NoError(t, g.AddTransaction(child, childDesc))

	// Trigger package identification to compute aggregate metrics.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify package fee rate is calculated correctly as
	// aggregate fees over aggregate size, resulting in a higher
	// rate than the parent alone.
	require.Len(t, packages, 1)
	pkg := packages[0]

	require.Equal(t, int64(1000), pkg.TotalFees)
	require.Equal(t, int64(200), pkg.TotalSize)
	require.Equal(t, int64(5000), pkg.FeeRate)
}

// TestComplexPackageIdentification verifies that highly connected
// transaction graphs with multiple roots and convergent paths are
// unified into a single package. This tests the package identification
// algorithm's ability to trace all spending relationships, which is
// critical for package relay to ensure complete package submission.
func TestComplexPackageIdentification(t *testing.T) {
	g := New(DefaultConfig())

	// Create a complex graph with multiple roots and convergent
	// spending patterns to test advanced package detection:
	//        root1    root2
	//         |  \    /  |
	//         |   mid1   |
	//         |  /    \  |
	//       child1   child2

	root1, root1Desc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(root1, root1Desc))

	root2, root2Desc := createTestTx(nil, 2)
	require.NoError(t, g.AddTransaction(root2, root2Desc))

	// Create middle transaction spending from both roots, forming
	// a convergence point.
	mid1, mid1Desc := createTestTx([]wire.OutPoint{
		{Hash: *root1.Hash(), Index: 1},
		{Hash: *root2.Hash(), Index: 0},
	}, 2)
	require.NoError(t, g.AddTransaction(mid1, mid1Desc))

	// Create children spending from both direct roots and the
	// middle transaction to form complex dependencies.
	child1, child1Desc := createTestTx([]wire.OutPoint{
		{Hash: *root1.Hash(), Index: 0},
		{Hash: *mid1.Hash(), Index: 0},
	}, 1)
	require.NoError(t, g.AddTransaction(child1, child1Desc))

	child2, child2Desc := createTestTx([]wire.OutPoint{
		{Hash: *root2.Hash(), Index: 1},
		{Hash: *mid1.Hash(), Index: 1},
	}, 1)
	require.NoError(t, g.AddTransaction(child2, child2Desc))

	// Trigger package identification to unify connected subgraph.
	packages, err := g.IdentifyPackages()
	require.NoError(t, err)

	// Verify all five transactions are unified into one standard
	// package with non-tree topology due to convergence points.
	require.Len(t, packages, 1)
	pkg := packages[0]

	require.Equal(t, PackageTypeStandard, pkg.Type)
	require.Len(t, pkg.Transactions, 5)
	require.Equal(t, 2, pkg.Topology.MaxDepth)
	require.False(t, pkg.Topology.IsLinear)
	// Convergent paths violate tree property.
	require.False(t, pkg.Topology.IsTree)
}

// TestEmptyPackageIdentification verifies that package identification
// on an empty graph returns an empty list rather than failing. This
// validates the edge case handling for package relay initialization.
func TestEmptyPackageIdentification(t *testing.T) {
	g := New(DefaultConfig())

	packages, err := g.IdentifyPackages()
	require.NoError(t, err)
	require.Empty(t, packages)
}

// TestSingleTransactionPackage verifies that isolated transactions
// can be packaged individually with correct topology properties. This
// ensures single-transaction packages are properly handled during
// package relay, even though they provide no CPFP benefit.
func TestSingleTransactionPackage(t *testing.T) {
	g := New(DefaultConfig())

	// Create an isolated transaction with no dependencies to test
	// the degenerate single-node package case.
	tx, desc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(tx, desc))

	// Retrieve the node for manual package construction.
	node, exists := g.GetNode(*tx.Hash())
	require.True(t, exists)

	// Create a package containing only this single transaction.
	pkg, err := g.CreatePackage([]*TxGraphNode{node})
	require.NoError(t, err)
	require.NotNil(t, pkg)

	// Verify the single-transaction package has correct topology
	// properties (depth 0, linear, tree).
	require.Len(t, pkg.Transactions, 1)
	require.Equal(t, PackageTypeStandard, pkg.Type)
	require.Equal(t, 0, pkg.Topology.MaxDepth)
	require.Equal(t, 1, pkg.Topology.MaxWidth)
	require.True(t, pkg.Topology.IsLinear)
	require.True(t, pkg.Topology.IsTree)
}

// TestGetPackage verifies that packages can be retrieved by any
// transaction hash within the package, and that lookups fail for
// transactions not in packages. This supports package relay by
// enabling efficient package discovery given any member transaction.
func TestGetPackage(t *testing.T) {
	g := New(DefaultConfig())

	// Create a simple package to test package lookup functionality.
	parent, parentDesc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(child, childDesc))

	packages, err := g.IdentifyPackages()
	require.NoError(t, err)
	require.Len(t, packages, 1)

	// Verify package can be retrieved using parent transaction
	// hash, demonstrating reverse lookup capability.
	pkg, err := g.GetPackage(*parent.Hash())
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.Equal(t, packages[0].ID, pkg.ID)

	// Verify package can also be retrieved using child transaction
	// hash, showing any package member can be used as lookup key.
	pkg, err = g.GetPackage(*child.Hash())
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.Equal(t, packages[0].ID, pkg.ID)

	// Verify lookup fails for transaction not in any package,
	// properly handling missing keys.
	orphan := wire.NewMsgTx(1)
	orphanHash := orphan.TxHash()
	pkg, err = g.GetPackage(orphanHash)
	require.Error(t, err)
	require.Nil(t, pkg)
}

// TestValidatePackage verifies that package validation detects
// malformed packages including nil packages, empty packages, and
// packages with inconsistent topology metadata. This is critical for
// package relay because invalid packages must be rejected before
// network propagation to prevent DoS attacks or relay failures.
func TestValidatePackage(t *testing.T) {
	g := New(DefaultConfig())

	// Create a well-formed package for baseline validation test.
	parent, parentDesc := createTestTx(nil, 1)
	require.NoError(t, g.AddTransaction(parent, parentDesc))

	child, childDesc := createTestTx(
		[]wire.OutPoint{{Hash: *parent.Hash(), Index: 0}}, 1)
	require.NoError(t, g.AddTransaction(child, childDesc))

	parentNode, _ := g.GetNode(*parent.Hash())
	childNode, _ := g.GetNode(*child.Hash())

	pkg, err := g.CreatePackage([]*TxGraphNode{parentNode, childNode})
	require.NoError(t, err)

	// Verify that a properly constructed package passes validation.
	err = g.ValidatePackage(pkg)
	require.NoError(t, err)

	// Verify nil package is rejected to prevent nil pointer errors.
	err = g.ValidatePackage(nil)
	require.Error(t, err)

	// Verify package with inconsistent topology is rejected,
	// preventing relay of corrupted package metadata.
	pkg.Topology.IsTree = false
	pkg.Topology.MaxDepth = 10
	err = g.ValidatePackage(pkg)
	require.Error(t, err)

	// Verify empty package is rejected as it provides no value for
	// package relay.
	emptyPkg := &TxPackage{
		Transactions: make(map[chainhash.Hash]*TxGraphNode),
	}
	err = g.ValidatePackage(emptyPkg)
	require.Error(t, err)
}
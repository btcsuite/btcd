package txgraph

import (
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/mock"
)

// MockPackageAnalyzer is a mock implementation of PackageAnalyzer for testing.
type MockPackageAnalyzer struct {
	mock.Mock
}

// IsTRUCTransaction checks if transaction is TRUC.
func (m *MockPackageAnalyzer) IsTRUCTransaction(tx *wire.MsgTx) bool {
	args := m.Called(tx)
	return args.Bool(0)
}

// HasEphemeralDust checks if transaction has ephemeral dust outputs.
func (m *MockPackageAnalyzer) HasEphemeralDust(tx *wire.MsgTx) bool {
	args := m.Called(tx)
	return args.Bool(0)
}

// IsZeroFee checks if transaction has zero fees.
func (m *MockPackageAnalyzer) IsZeroFee(desc *TxDesc) bool {
	args := m.Called(desc)
	return args.Bool(0)
}

// ValidateTRUCPackage validates TRUC package topology rules.
func (m *MockPackageAnalyzer) ValidateTRUCPackage(nodes []*TxGraphNode) bool {
	args := m.Called(nodes)
	return args.Bool(0)
}

// ValidateEphemeralPackage validates ephemeral dust package rules.
func (m *MockPackageAnalyzer) ValidateEphemeralPackage(
	nodes []*TxGraphNode) bool {

	args := m.Called(nodes)
	return args.Bool(0)
}

// AnalyzePackageType determines the type of package based on its structure.
func (m *MockPackageAnalyzer) AnalyzePackageType(
	nodes []*TxGraphNode) PackageType {

	args := m.Called(nodes)
	return args.Get(0).(PackageType)
}


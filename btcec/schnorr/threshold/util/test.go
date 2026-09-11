package util

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// TODO(aakselrod): Move to chainhash package?

	ENC_DECKEY_MSG_TAG = BIP_DKG_TAG + "encpedpop deckey"
)

type DKGOutput struct {
	SecShare        *btcec.PrivateKey
	ThresholdPubKey *btcec.PublicKey
	PubShares       []*btcec.PublicKey
}

type SimFunc func(*testing.T, [][32]byte, int, bool) ([]*DKGOutput, [][]byte)

type DKGTestCase struct {
	Name          string
	SimFunc       SimFunc
	Investigation bool
	Recovery      bool
}

type simFuncDescriptor struct {
	name                  string
	simFunc               SimFunc
	supportsInvestigation bool
	supportsRecovery      bool
}

var simFuncs []simFuncDescriptor

func RegisterSimFunc(name string, simFunc SimFunc,
	supportsInvestigation, supportsRecovery bool) {

	simFuncs = append(simFuncs, simFuncDescriptor{
		name:                  name,
		simFunc:               simFunc,
		supportsInvestigation: supportsInvestigation,
		supportsRecovery:      supportsRecovery,
	})
}

func GetDKGTests() []DKGTestCase {
	var testCases []DKGTestCase

	for _, simFunc := range simFuncs {
		testCases = append(testCases, DKGTestCase{
			Name:     simFunc.name,
			SimFunc:  simFunc.simFunc,
			Recovery: simFunc.supportsRecovery,
		})

		if simFunc.supportsInvestigation {
			testCases = append(testCases, DKGTestCase{
				Name:          simFunc.name,
				SimFunc:       simFunc.simFunc,
				Investigation: true,
				Recovery:      simFunc.supportsRecovery,
			})
		}
	}

	return testCases
}

func EncPedPopTestKeys(seed [32]byte) (*btcec.PrivateKey, *btcec.PublicKey) {
	keyHash := chainhash.TaggedHash([]byte(ENC_DECKEY_MSG_TAG), seed[:])
	return btcec.PrivKeyFromBytes(keyHash[:])
}

package silentpayments

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcec/v2"
)

var (
	testVectorFileName = "test-vectors.json"

	testdataDir = "testdata"
)

type TestVector struct {
	Comment string `json:"comment"`

	Sending []*Sending `json:"sending"`

	Receiving []*Receiving `json:"receiving"`
}

type Sending struct {
	Given *SendingGiven `json:"given"`

	Expected *SendingExpected `json:"expected"`
}

type SendingGiven struct {
	Vin []*VIn `json:"vin"`

	Recipients []string `json:"recipients"`
}

type VIn struct {
	Txid        string   `json:"txid"`
	Vout        uint32   `json:"vout"`
	ScriptSig   string   `json:"scriptSig"`
	TxInWitness string   `json:"txinwitness"`
	PrevOut     *PrevOut `json:"prevout"`
	PrivateKey  string   `json:"private_key"`
}

type PrevOut struct {
	ScriptPubKey *ScriptPubKey `json:"scriptPubKey"`
}

type ScriptPubKey struct {
	Hex string `json:"hex,omitempty"`
}

type SendingExpected struct {
	Outputs    [][]string `json:"outputs"`
	NumOutputs uint32     `json:"n_outputs"`
}

type Receiving struct {
	Given    *ReceivingGiven    `json:"given"`
	Expected *ReceivingExpected `json:"expected"`
}

type ReceivingGiven struct {
	Vin         []*VIn       `json:"vin"`
	Outputs     []string     `json:"outputs"`
	KeyMaterial *KeyMaterial `json:"key_material"`
	Labels      []uint32     `json:"labels"`
}

type KeyMaterial struct {
	ScanPrivKey  string `json:"scan_priv_key"`
	SpendPrivKey string `json:"spend_priv_key"`
}

func (k *KeyMaterial) Parse() (*btcec.PrivateKey, *btcec.PrivateKey, error) {
	scanPrivKeyBytes, err := hex.DecodeString(k.ScanPrivKey)
	if err != nil {
		return nil, nil, err
	}

	spendPrivKeyBytes, err := hex.DecodeString(k.SpendPrivKey)
	if err != nil {
		return nil, nil, err
	}

	scanPrivKey, _ := btcec.PrivKeyFromBytes(scanPrivKeyBytes)
	spendPrivKey, _ := btcec.PrivKeyFromBytes(spendPrivKeyBytes)

	return scanPrivKey, spendPrivKey, nil
}

type ReceivingExpected struct {
	Addresses  []string  `json:"addresses"`
	Outputs    []*Output `json:"outputs"`
	NumOutputs uint32    `json:"n_outputs"`
}

type Output struct {
	PrivKeyTweak string `json:"priv_key_tweak"`
	PubKey       string `json:"pub_key"`
	Signature    string `json:"signature"`
}

// ReadTestVectors reads the test vectors from the test vector file.
func ReadTestVectors() ([]*TestVector, error) {
	// Open the test vector file.
	file, err := os.Open(filepath.Join(testdataDir, testVectorFileName))
	if err != nil {
		return nil, err
	}

	// Decode the test vectors.
	var testVectors []*TestVector
	if err := json.NewDecoder(file).Decode(&testVectors); err != nil {
		return nil, err
	}

	return testVectors, nil
}

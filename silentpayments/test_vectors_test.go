package silentpayments

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

var (
	spTestVectorFileName = "bip-0352_send_and_receive_test_vectors.json"

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

	Recipients []*Recipient `json:"recipients"`
}

// Recipient is a silent payment recipient in the sending test vectors. The
// official vectors changed this from a bare address string to an object that
// also carries the decomposed scan and spend public keys.
type Recipient struct {
	Address     string `json:"address"`
	ScanPubKey  string `json:"scan_pub_key"`
	SpendPubKey string `json:"spend_pub_key"`

	// Count is the number of outputs that should be sent to this recipient.
	// It is used to test the per-group recipient limit without bloating the
	// vector file. An unset count means a single output.
	Count int `json:"count"`
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

// ParsePubKey parses the public key of the output.
func (o *Output) ParsePubKey() (*btcec.PublicKey, error) {
	pubKeyBytes, err := hex.DecodeString(o.PubKey)
	if err != nil {
		return nil, err
	}

	return schnorr.ParsePubKey(pubKeyBytes)
}

// ReadTestVectors reads the test vectors from the test vector file.
func ReadTestVectors() ([]*TestVector, error) {
	// Open the test vector file.
	file, err := os.Open(filepath.Join(testdataDir, spTestVectorFileName))
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

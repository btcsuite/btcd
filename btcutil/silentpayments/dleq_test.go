package silentpayments

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

var (
	dleqGenerateTestVectorFileName = "test-vectors-dleq-generate.json"
	dleqVerifyTestVectorFileName   = "test-vectors-dleq-verify.json"
)

func TestDLEQProof(t *testing.T) {
	vectors, err := ReadDLEQGenerateTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		t.Run(vector.Comment, func(tt *testing.T) {
			G, a, B, r, m, p, valid := vector.Parse(tt)
			result, err := DLEQProof(a, B, G, r, m)

			if !valid {
				require.Error(tt, err)
				return
			}

			require.NoError(tt, err)

			require.Equal(tt, p, result)
		})
	}
}

func TestDLEQVerify(t *testing.T) {
	vectors, err := ReadDLEQVerifyTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		t.Run(vector.Comment, func(tt *testing.T) {
			G, A, B, C, proof, m, valid := vector.Parse(tt)
			result := DLEQVerify(A, B, C, G, proof, m)

			require.Equal(tt, valid, result)
		})
	}
}

type DLEQGenerateVector struct {
	PointG      string `json:"point_g"`
	ScalarA     string `json:"scalar_a"`
	PointB      string `json:"point_b"`
	AuxRandR    string `json:"aux_rand_r"`
	Message     string `json:"message"`
	ResultProof string `json:"result_proof"`
	Comment     string `json:"comment"`
}

func (g *DLEQGenerateVector) Parse(t *testing.T) (*btcec.PublicKey,
	*btcec.PrivateKey, *btcec.PublicKey, [32]byte, *[32]byte, [64]byte,
	bool) {

	pubKeyG := decodePubKey(t, g.PointG)
	privKeyA := decodePrivKey(t, g.ScalarA)
	var pubKeyB *btcec.PublicKey
	if g.PointB == "INFINITY" {
		zero := btcec.FieldVal{}
		pubKeyB = btcec.NewPublicKey(&zero, &zero)
	} else {
		pubKeyB = decodePubKey(t, g.PointB)
	}
	auxRandR := decode32Array(t, g.AuxRandR)
	message := decode32Array(t, g.Message)

	var (
		valid bool
		proof [64]byte
	)
	if g.ResultProof != "INVALID" {
		proof = decode64Array(t, g.ResultProof)
		valid = true
	}

	return pubKeyG, privKeyA, pubKeyB, auxRandR, &message, proof, valid
}

type DLEQVerifyVector struct {
	PointG        string `json:"point_g"`
	PointA        string `json:"point_a"`
	PointB        string `json:"point_b"`
	PointC        string `json:"point_c"`
	Proof         string `json:"proof"`
	Message       string `json:"message"`
	ResultSuccess string `json:"result_success"`
	Comment       string `json:"comment"`
}

func (v *DLEQVerifyVector) Parse(t *testing.T) (*btcec.PublicKey,
	*btcec.PublicKey, *btcec.PublicKey, *btcec.PublicKey, [64]byte,
	*[32]byte, bool) {

	pubKeyG := decodePubKey(t, v.PointG)
	pubKeyA := decodePubKey(t, v.PointA)
	pubKeyB := decodePubKey(t, v.PointB)
	pubKeyC := decodePubKey(t, v.PointC)
	proof := decode64Array(t, v.Proof)
	message := decode32Array(t, v.Message)
	success := v.ResultSuccess == "TRUE"

	return pubKeyG, pubKeyA, pubKeyB, pubKeyC, proof, &message, success
}

// ReadDLEQGenerateTestVectors reads the DLEQ generate test vectors from the
// test vector file.
func ReadDLEQGenerateTestVectors() ([]*DLEQGenerateVector, error) {
	// Open the test vector file.
	file, err := os.Open(filepath.Join(
		testdataDir, dleqGenerateTestVectorFileName,
	))
	if err != nil {
		return nil, err
	}

	// Decode the test vectors.
	var testVectors []*DLEQGenerateVector
	if err := json.NewDecoder(file).Decode(&testVectors); err != nil {
		return nil, err
	}

	return testVectors, nil
}

// ReadDLEQVerifyTestVectors reads the DLEQ verify test vectors from the
// test vector file.
func ReadDLEQVerifyTestVectors() ([]*DLEQVerifyVector, error) {
	// Open the test vector file.
	file, err := os.Open(filepath.Join(
		testdataDir, dleqVerifyTestVectorFileName,
	))
	if err != nil {
		return nil, err
	}

	// Decode the test vectors.
	var testVectors []*DLEQVerifyVector
	if err := json.NewDecoder(file).Decode(&testVectors); err != nil {
		return nil, err
	}

	return testVectors, nil
}

func decodePubKey(t *testing.T, str string) *btcec.PublicKey {
	pubKeyBytes, err := hex.DecodeString(str)
	require.NoError(t, err)

	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	require.NoError(t, err)

	return pubKey
}

func decode32Array(t *testing.T, str string) [32]byte {
	rawBytes, err := hex.DecodeString(str)
	require.NoError(t, err)

	var array [32]byte
	copy(array[:], rawBytes)

	return array
}

func decode64Array(t *testing.T, str string) [64]byte {
	rawBytes, err := hex.DecodeString(str)
	require.NoError(t, err)

	var array [64]byte
	copy(array[:], rawBytes)

	return array
}

func decodePrivKey(t *testing.T, str string) *btcec.PrivateKey {
	privKeyBytes, err := hex.DecodeString(str)
	require.NoError(t, err)

	privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)

	return privKey
}

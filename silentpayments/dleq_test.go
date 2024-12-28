package silentpayments

import (
	"encoding/csv"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

var (
	dleqGenerateTestVectorFileName = "bip-0374_test_vectors_" +
		"generate_proof.csv"
	dleqVerifyTestVectorFileName = "bip-0374_test_vectors_" +
		"verify_proof.csv"
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

// TestCreateShare tests that CreateShare produces the correct ECDH share for a
// given input private key and recipient scan key, together with a DLEQ proof
// that verifies against that share.
//
// In BIP-374/BIP-352 terms: a is the input's private key and A = a·G is its
// public key, the scan key is B = b·G, and the resulting share is the ECDH
// secret C = a·B.
func TestCreateShare(t *testing.T) {
	t.Parallel()

	G := Generator()

	testCases := []struct {
		name     string
		inputKey string
		scanKey  string
	}{{
		name:     "small distinct keys",
		inputKey: "02",
		scanKey:  "03",
	}, {
		name:     "larger distinct keys",
		inputKey: "deadbeef",
		scanKey:  "cafebabe",
	}, {
		name:     "input key of one",
		inputKey: "01",
		scanKey:  "1337c0de",
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inputPriv := decodePrivKey(t, tc.inputKey)
			scanPriv := decodePrivKey(t, tc.scanKey)
			scanPub := scanPriv.PubKey()

			share, proof, err := CreateShare(inputPriv, scanPub)
			require.NoError(t, err)
			require.NotNil(t, share)
			require.True(t, share.IsOnCurve())

			// The share must be the ECDH secret C = a·B.
			expected := ScalarMult(inputPriv.Key, scanPub)
			require.True(t, expected.IsEqual(share))

			// ECDH symmetry: the receiver computes the same secret
			// from its scan key and the input public key, i.e.
			// b·A = b·(a·G) = a·(b·G) = a·B. This independently
			// confirms the share is the correct shared secret.
			inputPub := inputPriv.PubKey()
			recvSide := ScalarMult(scanPriv.Key, inputPub)
			require.True(t, recvSide.IsEqual(share))

			// The DLEQ proof must verify for A, B, C and G.
			require.True(t, DLEQVerify(
				inputPub, scanPub, share, G, proof, nil,
			))

			// A tampered proof must not verify.
			tampered := proof
			tampered[0] ^= 0x01
			require.False(t, DLEQVerify(
				inputPub, scanPub, share, G, tampered, nil,
			))

			// The proof must not verify against a different share.
			wrongShare := ScalarMult(inputPriv.Key, inputPub)
			require.False(t, DLEQVerify(
				inputPub, scanPub, wrongShare, G, proof, nil,
			))
		})
	}
}

type DLEQGenerateVector struct {
	PointG      string
	ScalarA     string
	PointB      string
	AuxRandR    string
	Message     string
	ResultProof string
	Comment     string
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
	message := decodeOptional32Array(t, g.Message)

	var (
		valid bool
		proof [64]byte
	)
	if g.ResultProof != "INVALID" {
		proof = decode64Array(t, g.ResultProof)
		valid = true
	}

	return pubKeyG, privKeyA, pubKeyB, auxRandR, message, proof, valid
}

type DLEQVerifyVector struct {
	PointG        string
	PointA        string
	PointB        string
	PointC        string
	Proof         string
	Message       string
	ResultSuccess string
	Comment       string
}

func (v *DLEQVerifyVector) Parse(t *testing.T) (*btcec.PublicKey,
	*btcec.PublicKey, *btcec.PublicKey, *btcec.PublicKey, [64]byte,
	*[32]byte, bool) {

	pubKeyG := decodePubKey(t, v.PointG)
	pubKeyA := decodePubKey(t, v.PointA)
	pubKeyB := decodePubKey(t, v.PointB)
	pubKeyC := decodePubKey(t, v.PointC)
	proof := decode64Array(t, v.Proof)
	message := decodeOptional32Array(t, v.Message)
	success := v.ResultSuccess == "TRUE"

	return pubKeyG, pubKeyA, pubKeyB, pubKeyC, proof, message, success
}

// ReadDLEQGenerateTestVectors reads the DLEQ generate test vectors from the
// test vector file. The official BIP-374 vectors are distributed as CSV with
// the columns: index, point_G, scalar_a, point_B, auxrand_r, message,
// result_proof, comment.
func ReadDLEQGenerateTestVectors() ([]*DLEQGenerateVector, error) {
	records, err := readCSVRecords(dleqGenerateTestVectorFileName)
	if err != nil {
		return nil, err
	}

	testVectors := make([]*DLEQGenerateVector, 0, len(records))
	for _, r := range records {
		testVectors = append(testVectors, &DLEQGenerateVector{
			PointG:      r[1],
			ScalarA:     r[2],
			PointB:      r[3],
			AuxRandR:    r[4],
			Message:     r[5],
			ResultProof: r[6],
			Comment:     r[7],
		})
	}

	return testVectors, nil
}

// ReadDLEQVerifyTestVectors reads the DLEQ verify test vectors from the test
// vector file. The official BIP-374 vectors are distributed as CSV with the
// columns: index, point_G, point_A, point_B, point_C, proof, message,
// result_success, comment.
func ReadDLEQVerifyTestVectors() ([]*DLEQVerifyVector, error) {
	records, err := readCSVRecords(dleqVerifyTestVectorFileName)
	if err != nil {
		return nil, err
	}

	testVectors := make([]*DLEQVerifyVector, 0, len(records))
	for _, r := range records {
		testVectors = append(testVectors, &DLEQVerifyVector{
			PointG:        r[1],
			PointA:        r[2],
			PointB:        r[3],
			PointC:        r[4],
			Proof:         r[5],
			Message:       r[6],
			ResultSuccess: r[7],
			Comment:       r[8],
		})
	}

	return testVectors, nil
}

// readCSVRecords opens a CSV test vector file in the testdata directory and
// returns all data rows, with the header row stripped.
func readCSVRecords(fileName string) ([][]string, error) {
	file, err := os.Open(filepath.Join(testdataDir, fileName))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, err
	}

	// Drop the header row, if present.
	if len(records) > 0 {
		records = records[1:]
	}

	return records, nil
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

// decodeOptional32Array decodes an optional 32-byte hex value. The vectors use
// an empty string to mean "no message", which the BIP-374 functions expect as a
// nil pointer rather than a pointer to a zero array.
func decodeOptional32Array(t *testing.T, str string) *[32]byte {
	if str == "" {
		return nil
	}

	array := decode32Array(t, str)

	return &array
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

// TestDLEQVerifyRejects exercises DLEQVerify's rejection paths that the BIP-374
// vectors don't cover: a point at infinity for any of A/B/C/G, and an s value
// greater than or equal to the secp256k1 group order.
func TestDLEQVerifyRejects(t *testing.T) {
	t.Parallel()

	G := Generator()
	a := decodePrivKey(t, "aa")
	scanPriv := decodePrivKey(t, "bb")
	B := scanPriv.PubKey()

	C, proof, err := CreateShare(a, B)
	require.NoError(t, err)
	A := a.PubKey()

	// Sanity check: the unmodified proof verifies.
	require.True(t, DLEQVerify(A, B, C, G, proof, nil))

	// A point at infinity for any of A, B, C, G must be rejected.
	inf := btcec.NewPublicKey(&btcec.FieldVal{}, &btcec.FieldVal{})
	require.False(t, DLEQVerify(inf, B, C, G, proof, nil))
	require.False(t, DLEQVerify(A, inf, C, G, proof, nil))
	require.False(t, DLEQVerify(A, B, inf, G, proof, nil))
	require.False(t, DLEQVerify(A, B, C, inf, proof, nil))

	// An s value equal to the group order (s >= n) must be rejected.
	const orderN = "fffffffffffffffffffffffffffffffe" +
		"baaedce6af48a03bbfd25e8cd0364141"
	order := decode32Array(t, orderN)
	var sOverflow [64]byte
	copy(sOverflow[:32], proof[:32])
	copy(sOverflow[32:], order[:])
	require.False(t, DLEQVerify(A, B, C, G, sOverflow, nil))
}

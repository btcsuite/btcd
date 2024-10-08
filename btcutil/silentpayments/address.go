package silentpayments

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Version is the address version.
type Version byte

const (
	// Version0 is the address version for the first version of the address
	// format.
	Version0 Version = 0

	// MainNetHRP is the human-readable part for mainnet addresses.
	MainNetHRP = "sp"

	// TestNetHRP is the human-readable part for testnet addresses.
	TestNetHRP = "tsp"
	
	// pubKeyLength is the length of a compressed public key.
	pubKeyLength = btcec.PubKeyBytesLenCompressed
)

var (
	// ErrUnsupportedNet is returned when trying to create an address for an
	// unsupported network.
	ErrUnsupportedNet = errors.New("unsupported network")

	// TagBIP0352Label is the BIP-0352 tag for a label.
	TagBIP0352Label = []byte("BIP0352/Label")
)

// Address is a struct that holds the public keys and tweak used to generate a
// silent payment address.
type Address struct {
	// Hrp is the human-readable part for the address.
	Hrp string

	// Version is the address version.
	Version Version

	// ScanKey is the public scan key.
	ScanKey btcec.PublicKey

	// SpendKey is the public spend key.
	SpendKey btcec.PublicKey

	// LabelTweak is an optional tweak to apply to the spend key. This is
	// calculated using:
	// 	tweak = TaggedHash_BIP0352/Label(scanPrivKey || m)
	LabelTweak *btcec.ModNScalar
}

// NewAddress creates a new silent payment address.
func NewAddress(hrp string, scanKey, spendKey btcec.PublicKey,
	labelTweak *btcec.ModNScalar) *Address {

	return &Address{
		Hrp:        hrp,
		ScanKey:    scanKey,
		SpendKey:   spendKey,
		LabelTweak: labelTweak,
	}
}

// NewAddressForNet creates a new silent payment address for a given network.
func NewAddressForNet(params *chaincfg.Params, scanKey,
	spendKey btcec.PublicKey, labelTweak *btcec.ModNScalar) (*Address,
	error) {

	var hrp string
	switch params.Name {
	case chaincfg.MainNetParams.Name:
		hrp = MainNetHRP

	case chaincfg.TestNet3Params.Name,
		chaincfg.SigNetParams.Name,
		chaincfg.RegressionNetParams.Name:

		hrp = TestNetHRP

	default:
		return nil, ErrUnsupportedNet
	}

	return NewAddress(hrp, scanKey, spendKey, labelTweak), nil
}

// TweakedSpendKey returns the spend key with the label tweak applied to it (if
// it exists).
func (a *Address) TweakedSpendKey() *btcec.PublicKey {
	// Create a copy, just in case.
	spendKey := a.SpendKey

	// If there is no tweak, just return the bare spend key.
	if a.LabelTweak == nil {
		return &spendKey
	}

	// Apply the tweak to the spend key now and return it.
	var bSpend, bTweak, result btcec.JacobianPoint
	spendKey.AsJacobian(&bSpend)

	// Calculate B_tweak = tweak * G.
	btcec.ScalarBaseMultNonConst(a.LabelTweak, &bTweak)

	// Calculate result = B_spend + B_tweak.
	btcec.AddNonConst(&bSpend, &bTweak, &result)

	result.ToAffine()
	return btcec.NewPublicKey(&result.X, &result.Y)
}

// EncodeAddress encodes the address into a bech32m string.
func (a *Address) EncodeAddress() string {
	// Copy the scan key into the payload unchanged.
	var payload [2 * pubKeyLength]byte
	copy(payload[:pubKeyLength],a.ScanKey.SerializeCompressed())
	copy(payload[pubKeyLength:],a.TweakedSpendKey().SerializeCompressed())

	// Group the address bytes into 5 bit groups, as this is what is used to
	// encode each character in the address string.
	converted, err := bech32.ConvertBits(payload[:], 8, 5, true)
	if err != nil {
		return ""
	}

	// Concatenate the address version and program, and encode the resulting
	// bytes using bech32m encoding.
	combined := make([]byte, len(converted)+1)
	combined[0] = byte(a.Version)
	copy(combined[1:], converted)

	addr, err := bech32.EncodeM(a.Hrp, combined)
	if err != nil {
		return ""
	}

	return addr
}

// DecodeAddress decodes a bech32m string into an address.
func DecodeAddress(addr string) (*Address, error) {
	// Spec: BIP173 imposes a 90 character limit for Bech32 segwit addresses
	// and limits versions to 0 through 16, whereas a silent payment address
	// requires at least 117 characters and allows versions up to 31.
	// Additionally, since higher versions may add to the data field, it is
	// recommended implementations use a limit of 1023 characters (see
	// BIP173: Checksum design for more details).
	if len(addr) > 1023 {
		return nil, bech32.ErrInvalidLength(len(addr))
	}

	hrp, data, bechVersion, err := bech32.DecodeNoLimitWithVersion(addr)
	if err != nil {
		return nil, err
	}

	if bechVersion != bech32.VersionM {
		return nil, errors.New("invalid bech32 version")
	}

	regrouped, err := bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return nil, err
	}

	// Spec: If the receiver's silent payment address version is:
	//    v0: check that the data part is exactly 66-bytes. Otherwise, fail.
	//    v1 through v30: read the first 66-bytes of the data part and
	//       discard the remaining bytes.
	//    v31: fail.
	addrVersion := data[0]
	switch {
	case addrVersion == 0:
		// Version zero addresses must be exactly 66 bytes.
		if len(regrouped) != 2*pubKeyLength {
			return nil, errors.New("invalid data length")
		}

	case addrVersion > 30:
		// Version 31 and above are not supported.
		return nil, fmt.Errorf("invalid silent address version: %v",
			addrVersion)

	default:
		// Any version between 1 and 29 is allowed, but we only read the
		// first 66 bytes.
		if len(regrouped) < 2*pubKeyLength {
			return nil, errors.New("invalid data length")
		}
	}

	scanKey, err := btcec.ParsePubKey(regrouped[:pubKeyLength])
	if err != nil {
		return nil, fmt.Errorf("error parsing scan key: %w", err)
	}

	spendKey, err := btcec.ParsePubKey(
		regrouped[pubKeyLength:2*pubKeyLength],
	)
	if err != nil {
		return nil, fmt.Errorf("error parsing spend key: %w", err)
	}

	return NewAddress(hrp, *scanKey, *spendKey, nil), nil
}

// LabelTweak calculates the label tweak for a given scan private key and m
// integer value.
func LabelTweak(scanPrivKey *btcec.PrivateKey, m uint32) *btcec.ModNScalar {
	var data [36]byte
	copy(data[:], scanPrivKey.Serialize())

	binary.BigEndian.PutUint32(data[32:], m)

	taggedHash := chainhash.TaggedHash(TagBIP0352Label, data[:])

	var scalar btcec.ModNScalar
	scalar.SetByteSlice(taggedHash[:])

	return &scalar
}

// GroupByScanKey groups a list of addresses by their scan key.
func GroupByScanKey(recipients []Address) [][]Address {
	groups := make(map[[33]byte][]Address)
	for _, recipient := range recipients {
		var key [33]byte
		copy(key[:], recipient.ScanKey.SerializeCompressed())

		groups[key] = append(groups[key], recipient)
	}

	idx := 0
	grouped := make([][]Address, len(groups))
	for _, group := range groups {
		grouped[idx] = group
		idx++
	}

	return grouped
}

package chilldkg

import (
	"errors"
	"fmt"
	"iter"

	"github.com/btcsuite/btcd/btcec/v2"
)

type errInvalidHostPubKey int

func (i errInvalidHostPubKey) Error() string {
	return fmt.Sprintf("Invalid host public key: %d", i)
}

var (
	// ErrThresholdOrCount is returned when the participant count or threshold is
	// invalid.
	ErrThresholdOrCount = errors.New("Threshold or participant count is invalid")
)

type ErrHostSecKey string

func (e ErrHostSecKey) Error() string {
	if e == "" {
		return "Invalid host secret key"
	}

	return string(e)
}

type ErrDuplicateHostPubKey struct {
	Participant1 int
	Participant2 int
}

func (e ErrDuplicateHostPubKey) Error() string {
	return fmt.Sprintf("Duplicate host public keys: %d, %d",
		e.Participant1, e.Participant2)
}

// hostPubKeyGen is used only in vector tests.
func hostPubKeyGen(hostSecKey []byte) (*btcec.PrivateKey, *btcec.PublicKey,
	error) {

	if len(hostSecKey) != 32 {
		return nil, nil, ErrHostSecKey("")
	}

	keyScalar := new(btcec.ModNScalar)
	overflows := keyScalar.SetByteSlice(hostSecKey)

	if overflows || keyScalar.IsZero() {
		return nil, nil, ErrHostSecKey("")
	}

	privKey := btcec.PrivKeyFromScalar(keyScalar)

	return privKey, privKey.PubKey(), nil
}

// signerCombinations takes a number of DKG participants N and a threshold T,
// and returns an iterator which yields every possible combination of signer
// IDs/indices of length T.
// This function is adapted from https://github.com/jhn-n/itertools,
// which is Copyright (c) 2026 jhn-n and distributed under the MIT license.
func signerCombinations(n, t int) iter.Seq[[]int] {

	return func(yield func([]int) bool) {
		if t > n || t < 0 {
			panic(fmt.Sprintf("invalid r value: permutating set of %v for subsets size %v", n, t))
		}

		indices := make([]int, t)
		for i := range t {
			indices[i] = i
		}

		tmp := make([]int, t)
		copy(tmp, indices)
		if !yield(tmp) {
			return
		}

		for {
			i := t - 1
			for i >= 0 {
				if indices[i] != i+n-t {
					break
				}
				i--
			}
			if i == -1 {
				return
			}
			indices[i] += 1
			for j := i + 1; j < t; j++ {
				indices[j] = indices[j-1] + 1
			}
			tmp := make([]int, t)
			copy(tmp, indices)
			if !yield(tmp) {
				return
			}

		}
	}
}

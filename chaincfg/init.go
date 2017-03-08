// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDuplicateVoteId = errors.New("duplicate vote id")
	ErrInvalidMask     = errors.New("invalid mask")
	ErrNotConsecutive  = errors.New("choices not consecutive")
	ErrTooManyChoices  = errors.New("too many choices")
	ErrInvalidIgnore   = errors.New("invalid ignore bits")
	ErrInvalidBits     = errors.New("invalid vote bits")
	ErrInvalidIsIgnore = errors.New("one and only one IsIgnore rule " +
		"violation")
	ErrInvalidIsNo      = errors.New("one and only one IsNo rule violation")
	ErrInvalidBothFlags = errors.New("IsNo and IsIgnore may not be both " +
		"set to true")
	ErrDuplicateChoiceId = errors.New("duplicate choice ID")
)

// bitsSet counts number of bits set.
// Proudly stolen from:
// https://graphics.stanford.edu/~seander/bithacks.html#CountBitsSetKernighan
func bitsSet(bits uint16) uint {
	c := uint(0)
	for v := bits; v != 0; c++ {
		v &= v - 1
	}
	return c
}

// consecOnes count consecutive 1 bits set.
func consecOnes(bits uint16) uint {
	c := uint(0)
	for v := bits; v != 0; c++ {
		v = v & (v << 1)
	}
	return c
}

// shift calculates the number of bits that need shifting to get to an index.
func shift(mask uint16) uint {
	shift := uint(0)
	for {
		if mask&0x0001 == 0x0001 {
			break
		}
		shift++
		mask >>= 1
	}
	return shift
}

func validateChoices(mask uint16, choices []Choice) error {
	var (
		isIgnore, isNo int
	)

	// Check that mask is consecutive.
	if consecOnes(mask) != bitsSet(mask) {
		return ErrInvalidMask
	}

	// Check bits and choice bounds.
	if len(choices) > 1<<bitsSet(mask) {
		return ErrTooManyChoices
	}

	dups := make(map[string]struct{})
	s := shift(mask)
	for index, choice := range choices {
		// Check that choice 0 is the ignore vote.
		if mask&choice.Bits == 0 && !choice.IsIgnore {
			return ErrInvalidIgnore
		}

		// Check mask bits.
		if mask&choice.Bits != choice.Bits {
			return ErrInvalidBits
		}

		// Check that index is consecutive.  This test is below the
		// Check mask bits one for testing reasons.  Leave it here.
		if uint16(index) != choice.Bits>>s {
			return ErrNotConsecutive
		}

		// Check that both flags aren't set to true.
		if choice.IsIgnore && choice.IsNo {
			return ErrInvalidBothFlags
		}

		// Count flags.
		if choice.IsIgnore {
			isIgnore++
		}
		if choice.IsNo {
			isNo++
		}

		// Check for duplicates.
		id := strings.ToLower(choice.Id)
		_, found := dups[id]
		if found {
			return ErrDuplicateChoiceId
		}
		dups[id] = struct{}{}
	}

	// Check that there is only one IsNo and IsIgnore flag set to true.
	if isIgnore != 1 {
		return ErrInvalidIsIgnore
	}
	if isNo != 1 {
		return ErrInvalidIsNo
	}

	return nil
}

func validateAgenda(vote Vote) error {
	return validateChoices(vote.Mask, vote.Choices)
}

func validateDeployments(deployments []ConsensusDeployment) (int, error) {
	dups := make(map[string]struct{})
	for index, deployment := range deployments {
		// Check for duplicates.
		id := strings.ToLower(deployment.Vote.Id)
		_, found := dups[id]
		if found {
			return index, ErrDuplicateVoteId
		}
		dups[id] = struct{}{}
	}

	return -1, nil
}

func validateAgendas() {
	for i := 0; i < 3; i++ {
		var params Params
		switch i {
		case 0:
			params = MainNetParams
		case 1:
			params = TestNetParams
		case 2:
			params = SimNetParams
		default:
			panic("invalid net")
		}

		for version, deployments := range params.Deployments {
			index, err := validateDeployments(deployments)
			if err != nil {
				e := fmt.Sprintf("invalid agenda on %v "+
					"version %v id %v: %v", params.Name,
					version, deployments[index].Vote.Id,
					err)
				panic(e)
			}

			for _, deployment := range deployments {
				err := validateAgenda(deployment.Vote)
				if err != nil {
					e := fmt.Sprintf("invalid agenda "+
						"on %v version %v id %v: %v",
						params.Name, version,
						deployment.Vote.Id, err)
					panic(e)
				}
			}
		}
	}
}

func init() {
	validateAgendas()
}

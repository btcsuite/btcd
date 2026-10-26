package frost

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidThreshold = errors.New("The threshold must be 1 <= t <= n.")

	ErrInvalidNumParticipants = fmt.Errorf("The number of participants "+
		"must be n <= %d.", MAX_PARTICIPANTS)

	ErrMismatchPubSharesIds = errors.New("The pubshares and ids lists " +
		"must have the same length")

	ErrDuplicateElements = errors.New("The ids list contains duplicate " +
		"elements.")

	ErrPubSharesThreshPKMismatch = errors.New("The provided key " +
		"material is incorrect: the public shares do not match the " +
		"threshold public key.")

	errSignersSecretShareValue = errors.New("The signer's secret share " +
		"value is out of range.")

	errTweakLength = errors.New("The tweak must be a 32-byte array.")
)

type ContributionType uint8

const (
	ContributionTypePubNonce ContributionType = iota
	ContributionTypeAggNonce
	ContributionTypeAggOtherNonce
	ContributionTypePSig
)

func (c ContributionType) String() string {
	switch c {
	case ContributionTypePubNonce:
		return "pubnonce"

	case ContributionTypeAggOtherNonce:
		return "aggothernonce"

	case ContributionTypeAggNonce:
		return "aggnonce"

	case ContributionTypePSig:
		return "psig"

	default:
		return "unknown"
	}
}

type ErrInvalidContribution struct {
	// SignerIndex is the signer index when the error is due to a known
	// signer, or -1 when there's an error due to an aggregated value.
	SignerIndex int

	// ContributionType is the type of invalid contribution.
	ContributionType
}

func (e ErrInvalidContribution) Error() string {
	return "Invalid contribution"
}

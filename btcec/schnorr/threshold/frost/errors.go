package frost

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

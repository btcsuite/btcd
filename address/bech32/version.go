package bech32

// ChecksumConst is a type that represents the currently defined bech32
// checksum constants.
type ChecksumConst int

const (
	// Version0Const is the original constant used in the checksum
	// verification for bech32.
	Version0Const ChecksumConst = 1

	// VersionMConst is the new constant used for bech32m checksum
	// verification.
	VersionMConst ChecksumConst = 0x2bc830a3
)

// Version defines the current set of bech32 versions.
type Version uint8

const (
	// Version0 defines the original bech version.
	Version0 Version = iota

	// VersionM is the new bech32 version defined in BIP-350, also known as
	// bech32m.
	VersionM

	// VersionUnknown denotes an unknown bech version.
	VersionUnknown
)

// VersionToConsts maps bech32 versions to the checksum constant to be used
// when encoding, and asserting a particular version when decoding.
var VersionToConsts = map[Version]ChecksumConst{
	Version0: Version0Const,
	VersionM: VersionMConst,
}

// ConstsToVersion maps a bech32 constant to the version it's associated with.
var ConstsToVersion = map[ChecksumConst]Version{
	Version0Const: Version0,
	VersionMConst: VersionM,
}

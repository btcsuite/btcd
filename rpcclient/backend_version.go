package rpcclient

import "strings"

// BackendVersion represents the version of the backend the client is currently
// connected to.
type BackendVersion uint8

const (
	// BitcoindPre19 represents a bitcoind version before 0.19.0.
	BitcoindPre19 BackendVersion = iota

	// BitcoindPre22 represents a bitcoind version equal to or greater than
	// 0.19.0 and smaller than 22.0.0.
	BitcoindPre22

	// BitcoindPre24 represents a bitcoind version equal to or greater than
	// 22.0.0 and smaller than 24.0.0.
	BitcoindPre24

	// BitcoindPre25 represents a bitcoind version equal to or greater than
	// 24.0.0 and smaller than 25.0.0.
	BitcoindPre25

	// BitcoindPre25 represents a bitcoind version equal to or greater than
	// 25.0.0.
	BitcoindPost25

	// Btcd represents a catch-all btcd version.
	Btcd
)

// String returns a human-readable backend version.
func (b BackendVersion) String() string {
	switch b {
	case BitcoindPre19:
		return "bitcoind 0.19 and below"

	case BitcoindPre22:
		return "bitcoind v0.19.0-v22.0.0"

	case BitcoindPre24:
		return "bitcoind v22.0.0-v24.0.0"

	case BitcoindPre25:
		return "bitcoind v24.0.0-v25.0.0"

	case BitcoindPost25:
		return "bitcoind v25.0.0 and above"

	case Btcd:
		return "btcd"

	default:
		return "unknown"
	}
}

const (
	// bitcoind19Str is the string representation of bitcoind v0.19.0.
	bitcoind19Str = "0.19.0"

	// bitcoind22Str is the string representation of bitcoind v22.0.0.
	bitcoind22Str = "22.0.0"

	// bitcoind24Str is the string representation of bitcoind v24.0.0.
	bitcoind24Str = "24.0.0"

	// bitcoind25Str is the string representation of bitcoind v25.0.0.
	bitcoind25Str = "25.0.0"

	// bitcoindVersionPrefix specifies the prefix included in every bitcoind
	// version exposed through GetNetworkInfo.
	bitcoindVersionPrefix = "/Satoshi:"

	// bitcoindVersionSuffix specifies the suffix included in every bitcoind
	// version exposed through GetNetworkInfo.
	bitcoindVersionSuffix = "/"
)

// parseBitcoindVersion parses the bitcoind version from its string
// representation.
func parseBitcoindVersion(version string) BackendVersion {
	// Trim the version of its prefix and suffix to determine the
	// appropriate version number.
	version = strings.TrimPrefix(
		strings.TrimSuffix(version, bitcoindVersionSuffix),
		bitcoindVersionPrefix,
	)
	switch {
	case version < bitcoind19Str:
		return BitcoindPre19

	case version < bitcoind22Str:
		return BitcoindPre22

	case version < bitcoind24Str:
		return BitcoindPre24

	case version < bitcoind25Str:
		return BitcoindPre25

	default:
		return BitcoindPost25
	}
}

package rpcclient

import "testing"

// TestUnmarshalGetBlockChainInfoResult ensures that the SoftForks and
// UnifiedSoftForks fields of GetBlockChainInfoResult are properly unmarshaled
// when using the expected backend version.
func TestUnmarshalGetBlockChainInfoResultSoftForks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    BackendVersion
		res        []byte
		compatible bool
	}{
		{
			name:       "bitcoind < 0.19.0 with separate softforks",
			version:    BitcoindPre19,
			res:        []byte(`{"softforks": [{"version": 2}]}`),
			compatible: true,
		},
		{
			name:       "bitcoind >= 0.19.0 with separate softforks",
			version:    BitcoindPost19,
			res:        []byte(`{"softforks": [{"version": 2}]}`),
			compatible: false,
		},
		{
			name:       "bitcoind < 0.19.0 with unified softforks",
			version:    BitcoindPre19,
			res:        []byte(`{"softforks": {"segwit": {"type": "bip9"}}}`),
			compatible: false,
		},
		{
			name:       "bitcoind >= 0.19.0 with unified softforks",
			version:    BitcoindPost19,
			res:        []byte(`{"softforks": {"segwit": {"type": "bip9"}}}`),
			compatible: true,
		},
	}

	for _, test := range tests {
		success := t.Run(test.name, func(t *testing.T) {
			// We'll start by unmarshaling the JSON into a struct.
			// The SoftForks and UnifiedSoftForks field should not
			// be set yet, as they are unmarshaled within a
			// different function.
			info, err := unmarshalPartialGetBlockChainInfoResult(test.res)
			if err != nil {
				t.Fatal(err)
			}
			if info.SoftForks != nil {
				t.Fatal("expected SoftForks to be empty")
			}
			if info.UnifiedSoftForks != nil {
				t.Fatal("expected UnifiedSoftForks to be empty")
			}

			// Proceed to unmarshal the softforks of the response
			// with the expected version. If the version is
			// incompatible with the response, then this should
			// fail.
			err = unmarshalGetBlockChainInfoResultSoftForks(
				info, test.version, test.res,
			)
			if test.compatible && err != nil {
				t.Fatalf("unable to unmarshal softforks: %v", err)
			}
			if !test.compatible && err == nil {
				t.Fatal("expected to not unmarshal softforks")
			}
			if !test.compatible {
				return
			}

			// If the version is compatible with the response, we
			// should expect to see the proper softforks field set.
			if test.version == BitcoindPost19 &&
				info.SoftForks != nil {
				t.Fatal("expected SoftForks to be empty")
			}
			if test.version == BitcoindPre19 &&
				info.UnifiedSoftForks != nil {
				t.Fatal("expected UnifiedSoftForks to be empty")
			}
		})
		if !success {
			return
		}
	}
}

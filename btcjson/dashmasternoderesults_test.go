package btcjson_test

import (
	"encoding/json"
	"github.com/dashevo/dashd-go/btcjson"
	"testing"
)

// TestDashMasternodeStatusResults ensures MasternodeStatusResults are unmarshalled correctly
func TestDashMasternodeStatusResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		result   btcjson.MasternodeStatusResult
	}{
		{
			name:     "masternode info",
			expected: `{"outpoint":"d1be3a1aa0b9516d06ed180607c168724c21d8ccf6c5a3f5983769830724c357-0","service":"45.32.237.76:19999","proTxHash":"04d06d16b3eca2f104ef9749d0c1c17d183eb1b4fe3a16808fd70464f03bcd63","collateralHash":"d1be3a1aa0b9516d06ed180607c168724c21d8ccf6c5a3f5983769830724c357","collateralIndex":0,"dmnState":{"service":"45.32.237.76:19999","registeredHeight":7402,"lastPaidHeight":59721,"PoSePenalty":0,"PoSeRevivedHeight":61915,"PoSeBanHeight":-1,"revocationReason":0,"ownerAddress":"yT8DDY5NkX4ZtBkUVz7y1RgzbakCnMPogh","votingAddress":"yMLrhooXyJtpV3R2ncsxvkrh6wRennNPoG","payoutAddress":"yTsGq4wV8WF5GKLaYV2C43zrkr2sfTtysT","pubKeyOperator":"02a2e2673109a5e204f8a82baf628bb5f09a8dfc671859e84d2661cae03e6c6e198a037e968253e94cd099d07b98e94e"},"state":"READY","status":"Ready"}`,
			result: btcjson.MasternodeStatusResult{
				Outpoint:        "d1be3a1aa0b9516d06ed180607c168724c21d8ccf6c5a3f5983769830724c357-0",
				Service:         "45.32.237.76:19999",
				ProTxHash:       "04d06d16b3eca2f104ef9749d0c1c17d183eb1b4fe3a16808fd70464f03bcd63",
				CollateralHash:  "d1be3a1aa0b9516d06ed180607c168724c21d8ccf6c5a3f5983769830724c357",
				CollateralIndex: 0,
				DMNState: btcjson.DMNState{
					Service:           "45.32.237.76:19999",
					RegisteredHeight:  7402,
					LastPaidHeight:    59721,
					PoSePenalty:       0,
					PoSeRevivedHeight: 61915,
					PoSeBanHeight:     -1,
					RevocationReason:  0,
					OwnerAddress:      "yT8DDY5NkX4ZtBkUVz7y1RgzbakCnMPogh",
					VotingAddress:     "yMLrhooXyJtpV3R2ncsxvkrh6wRennNPoG",
					PayoutAddress:     "yTsGq4wV8WF5GKLaYV2C43zrkr2sfTtysT",
					PubKeyOperator:    "02a2e2673109a5e204f8a82baf628bb5f09a8dfc671859e84d2661cae03e6c6e198a037e968253e94cd099d07b98e94e",
				},
				State:  "READY",
				Status: "Ready",
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		marshalled, err := json.Marshal(test.result)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if string(marshalled) != test.expected {
			t.Errorf("Test #%d (%s) unexpected marhsalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.expected)
			continue
		}
	}
}

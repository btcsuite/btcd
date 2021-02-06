package btcjson_test

import (
	"encoding/json"
	"github.com/dashevo/dashd-go/btcjson"
	"testing"
)

// TestDashQuorumSignResults ensures QuorumSignResults are unmarshalled correctly
func TestDashQuorumSignResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		result   btcjson.QuorumSignResult
	}{
		{
			name:     "quorum sign",
			expected: `{"llmqType":100,"quorumHash":"53d959f609a654cf4e5e3c083fd6c47b7ec6cb73af4ac7329149688337b8ef9a","quorumMember":2,"id":"0000000000000000000000000000000000000000000000000000000000000001","msgHash":"0000000000000000000000000000000000000000000000000000000000000002","signHash":"39458221939396a45a2e348caada646eabd52849990827d40e33eb1399097b3c","signature":"9716545a0c28ff70900a71fabbadf3c13e4ae562032122902405365f1ebf3da813c8a97d765eb8b167ff339c1638550c13822217cf06b609ba6a78f0035684ca7b4afdb7146ce74a30cfb6770f852aade8c27ffec67c79f85be31964573fb51c"}`,
			result: btcjson.QuorumSignResult{
				LLMQType:     100,
				QuorumHash:   "53d959f609a654cf4e5e3c083fd6c47b7ec6cb73af4ac7329149688337b8ef9a",
				QuorumMember: 2,
				ID:           "0000000000000000000000000000000000000000000000000000000000000001",
				MsgHash:      "0000000000000000000000000000000000000000000000000000000000000002",
				SignHash:     "39458221939396a45a2e348caada646eabd52849990827d40e33eb1399097b3c",
				Signature:    "9716545a0c28ff70900a71fabbadf3c13e4ae562032122902405365f1ebf3da813c8a97d765eb8b167ff339c1638550c13822217cf06b609ba6a78f0035684ca7b4afdb7146ce74a30cfb6770f852aade8c27ffec67c79f85be31964573fb51c",
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

// TestDashQuorumInfoResults ensures QuorumInfoResults are unmarshalled correctly
func TestDashQuorumInfoResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		result   btcjson.QuorumInfoResult
	}{
		{
			name:     "quorum info",
			expected: `{"height":264072,"type":"llmq_50_60","quorumHash":"000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9","minedBlock":"000006113a77b35a0ed606b08ecb8e37f1ac7e2d773c365bd07064a72ae9a61d","members":[{"proTxHash":"6c91363d97b286e921afb5cf7672c88a2f1614d36d32058c34bef8b44e026007","pubKeyOperator":"81749ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3121","valid":true,"pubKeyShare":"83349ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123"},{"proTxHash":"274ae6ab38ea0f3b8fe726b3e52d998443ba0d77e85d88c20d179d4fecd0b96e","pubKeyOperator":"0db6da5d8ee9fb8925f0818df7553062bf35ec9d62114144bc395980c29fcd06b738beca63faf265d7480106fc6cceea","valid":true,"pubKeyShare":"45549ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123"},{"proTxHash":"3ecdbedf3d9a13822f437a1f0c5ea44f290ab90f7c3bb42c1b5fd785b5f9596a","pubKeyOperator":"0634f8b926631cb2b14c81720c6130b3f6f5429da1c9dc9c33918b2474b7ffff239caa9b59c7b1a782565052232d052a","valid":true,"pubKeyShare":"67749ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123"}],"quorumPublicKey":"0644ff153b9b92c6a59e2adf4ef0b9836f7f6af05fe432ffdcb69bc9e300a2a70af4a8d9fc61323f6b81074d740033d2","secretKeyShare":"3da0d8f532309660f7f44aa0ed42c1569773b39c70f5771ce5604be77e50759e"}`,
			result: btcjson.QuorumInfoResult{
				Height:     264072,
				Type:       "llmq_50_60",
				QuorumHash: "000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",
				MinedBlock: "000006113a77b35a0ed606b08ecb8e37f1ac7e2d773c365bd07064a72ae9a61d",
				Members: []btcjson.QuorumMember{
					{
						ProTxHash:      "6c91363d97b286e921afb5cf7672c88a2f1614d36d32058c34bef8b44e026007",
						PubKeyOperator: "81749ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3121",
						Valid:          true,
						PubKeyShare:    "83349ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123",
					},
					{
						ProTxHash:      "274ae6ab38ea0f3b8fe726b3e52d998443ba0d77e85d88c20d179d4fecd0b96e",
						PubKeyOperator: "0db6da5d8ee9fb8925f0818df7553062bf35ec9d62114144bc395980c29fcd06b738beca63faf265d7480106fc6cceea",
						Valid:          true,
						PubKeyShare:    "45549ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123",
					},
					{
						ProTxHash:      "3ecdbedf3d9a13822f437a1f0c5ea44f290ab90f7c3bb42c1b5fd785b5f9596a",
						PubKeyOperator: "0634f8b926631cb2b14c81720c6130b3f6f5429da1c9dc9c33918b2474b7ffff239caa9b59c7b1a782565052232d052a",
						Valid:          true,
						PubKeyShare:    "67749ba8363e5c03e9d6318b0491e38305cf59d9d57cea2295a86ecfa696622571f266c28bacc78666e8b9b0fb2b3123",
					},
				},
				QuorumPublicKey: "0644ff153b9b92c6a59e2adf4ef0b9836f7f6af05fe432ffdcb69bc9e300a2a70af4a8d9fc61323f6b81074d740033d2",
				SecretKeyShare:  "3da0d8f532309660f7f44aa0ed42c1569773b39c70f5771ce5604be77e50759e",
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

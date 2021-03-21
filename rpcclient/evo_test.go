package rpcclient

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/dashevo/dashd-go/btcjson"
)

func TestBLS(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	gen, err := client.BLSGenerate()
	if err != nil {
		t.Fatal(err)
	}

	fs, err := client.BLSFromSecret(gen.Secret)
	if err != nil {
		t.Fatal(err)
	}

	if gen.Public != fs.Public {
		t.Fatal("public generated did not match fromsecret")
	}
	if gen.Secret != fs.Secret {
		t.Fatal("secret generated did not match fromsecret")
	}

	cli := &btcjson.BLSResult{}
	compareWithCliCommand(t, fs, cli, "bls", "fromsecret", gen.Secret)
}

func TestQuorumList(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.QuorumList()
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.QuorumListResult{}
	compareWithCliCommand(t, result, cli, "quorum", "list")
}

func TestQuorumInfo(t *testing.T) {
	quorumType := btcjson.LLMQType_400_60

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	list, err := client.QuorumList()
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Llmq400_60) == 0 {
		t.Fatal("list llmq_400_60 empty")
	}
	quorumHash := list.Llmq400_60[0]

	result, err := client.QuorumInfo(quorumType, quorumHash, false)
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.QuorumInfoResult{}
	compareWithCliCommand(t, result, cli, "quorum", "info", fmt.Sprint(quorumType), quorumHash)
}

func TestQuorumSelectQuorum(t *testing.T) {
	quorumType := btcjson.LLMQType_400_60
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.QuorumSelectQuorum(quorumType, requestID)
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.QuorumSelectQuorumResult{}
	compareWithCliCommand(t, result, cli, "quorum", "selectquorum", fmt.Sprint(quorumType), requestID)
}

func TestQuorumDKGStatus(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	for _, dl := range []btcjson.DetailLevel{
		btcjson.DetailLevelCounts,
		btcjson.DetailLevelIndexes,
		btcjson.DetailLevelMembersProTxHashes,
	} {
		t.Run(fmt.Sprint(dl), func(t *testing.T) {
			result, err := client.QuorumDKGStatus(dl)
			if err != nil {
				t.Fatal(err)
			}

			var cli interface{}
			switch result.(type) {
			case *btcjson.QuorumDKGStatusCountsResult:
				cli = &btcjson.QuorumDKGStatusCountsResult{}
			case *btcjson.QuorumDKGStatusIndexesResult:
				cli = &btcjson.QuorumDKGStatusIndexesResult{}
			case *btcjson.QuorumDKGStatusMembersProTxHashesResult:
				cli = &btcjson.QuorumDKGStatusMembersProTxHashesResult{}
			default:
				t.Fatalf("unknown type %T", result)
			}

			compareWithCliCommand(t, result, cli, "quorum", "dkgstatus", fmt.Sprint(dl))
		})
	}
}

func TestQuorumMemberOf(t *testing.T) {
	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.QuorumMemberOf(proTxHash, 0)
	if err != nil {
		t.Fatal(err)
	}

	cli := []btcjson.QuorumMemberOfResult{}
	compareWithCliCommand(t, &result, &cli, "quorum", "memberof", proTxHash)
}

var llmqTypes = map[string]btcjson.LLMQType{
	"llmq_50_60":  btcjson.LLMQType_50_60,
	"llmq_400_60": btcjson.LLMQType_400_60,
	"llmq_400_85": btcjson.LLMQType_400_85,
	"llmq_100_67": btcjson.LLMQType_100_67,
	"llmq_5_60":   btcjson.LLMQType_5_60,
}

func TestQuorumSign(t *testing.T) {
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	messageHash := "51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"
	submit := false

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"
	mo, err := client.QuorumMemberOf(proTxHash, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(mo) == 0 {
		t.Fatal("not a member of any quorums")
	}
	quorumHash := mo[0].QuorumHash
	quorumType, ok := llmqTypes[mo[0].Type]
	if !ok {
		t.Fatal("unknown quorum type", mo[0].Type)
	}

	result, err := client.QuorumSign(quorumType, requestID, messageHash, quorumHash, submit)
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.QuorumSignResultWithBool{}
	compareWithCliCommand(t, result, cli, "quorum", "sign", fmt.Sprint(quorumType), requestID, messageHash, quorumHash, strconv.FormatBool(submit))

	bl, err := client.QuorumSignSubmit(quorumType, requestID, messageHash, quorumHash)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("bool response:", bl)

}

func TestQuorumGetRecSig(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	quorumType := btcjson.LLMQType_400_60
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	messageHash := "51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"

	result, err := client.QuorumGetRecSig(quorumType, requestID, messageHash)
	if err != nil {
		t.Fatal(err)
	}

	cli := []btcjson.QuorumSignResult{}
	compareWithCliCommand(t, result, cli, "quorum", "getrecsig", fmt.Sprint(quorumType), requestID, messageHash)
}

func TestQuorumHasRecSig(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	quorumType := btcjson.LLMQType_400_60
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	messageHash := "51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"

	result, err := client.QuorumHasRecSig(quorumType, requestID, messageHash)
	if err != nil {
		t.Fatal(err)
	}

	if !result {
		t.Fatal("returned false")
	}

	var cli bool
	compareWithCliCommand(t, result, cli, "quorum", "hasrecsig", fmt.Sprint(quorumType), requestID, messageHash)
}

func TestQuorumsConflicting(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	quorumType := btcjson.LLMQType_400_60
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	messageHash := "51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"

	result, err := client.QuorumIsConflicting(quorumType, requestID, messageHash)
	if err != nil {
		t.Fatal(err)
	}

	if !result {
		t.Fatal("returned false")
	}

	var cli bool
	compareWithCliCommand(t, result, cli, "quorum", "isconflicting", fmt.Sprint(quorumType), requestID, messageHash)
}

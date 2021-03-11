package rpcclient

import (
	"fmt"
	"testing"

	"github.com/dashevo/dashd-go/btcjson"
)

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

func TestQuorumSign(t *testing.T) {
	quorumType := btcjson.LLMQType_400_60
	requestID := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	messageHash := "51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"
	quorumHash := "000000dc07d722238a994116c3395c334211d9864ff5b37c3be51d5fdda66223"
	submit := false

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.QuorumSign(quorumType, requestID, messageHash, quorumHash, submit)
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.MasternodeStatusResult{}
	compareWithCliCommand(t, result, cli, "quorum", "status")
}

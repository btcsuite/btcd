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

func TestProTxInfo(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"

	result, err := client.ProTxInfo(proTxHash)
	if err != nil {
		t.Fatal(err)
	}

	modify := func(i interface{}) {
		r, ok := i.(*btcjson.ProTxInfoResult)
		if !ok {
			t.Fatalf("cleanup function could not type cast from %T", i)
		}
		r.MetaInfo.LastOutboundAttemptElapsed = 0
	}

	var cli btcjson.ProTxInfoResult
	modifyThenCompareWithCliCommand(t, modify, result, &cli, "protx", "info", proTxHash)
}

func TestProTxList(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.ProTxList("", false, 0)
	if err != nil {
		t.Fatal(err)
	}

	rSlice, ok := result.([]string)
	if !ok {
		t.Fatalf("list was not a slice %T", result)
	}

	var cli []string
	compareWithCliCommand(t, &rSlice, &cli, "protx", "list")

	cmdType := btcjson.ProTxListTypeValid
	height := 415243
	result, err = client.ProTxList(cmdType, true, height)
	if err != nil {
		t.Fatal(err)
	}

	modify := func(i interface{}) {
		r, ok := i.(*[]btcjson.ProTxInfoResult)
		if !ok {
			t.Fatalf("cleanup function could not type cast from %T", i)
		}
		for _, ri := range *r {
			ri.MetaInfo.LastOutboundAttemptElapsed = 0
		}
	}

	rDetails, ok := result.([]btcjson.ProTxInfoResult)
	if !ok {
		t.Fatal("list was not an info slice")
	}

	var cliDetails []btcjson.ProTxInfoResult
	modifyThenCompareWithCliCommand(t, modify, &rDetails, &cliDetails, "protx", "list", string(cmdType), "true", strconv.Itoa(height))
}

func TestProTxDiff(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	baseBlock := 75000
	block := 76000

	result, err := client.ProTxDiff(baseBlock, block)
	if err != nil {
		t.Fatal(err)
	}

	var cli btcjson.ProTxDiffResult
	compareWithCliCommand(t, result, &cli, "protx", "diff", strconv.Itoa(baseBlock), strconv.Itoa(block))
}

func TestProTxUpdateService(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"
	ipAndPort := "144.202.120.18:19999"
	operatorKey := "084ceaabfe23865823aa696258245d8f94144fc33fb558528cd1742ef8f033d7b8c701d19cd6a561522c9e8d82bf7283"

	result, err := client.ProTxUpdateService(proTxHash, ipAndPort, operatorKey, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

func TestProTxUpdateRegistrar(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"
	operatorKey := "1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"
	votingAddress := "yP9kcJKUXt8BSfL8Pcj83fsMjBGhDAmBb6"
	payoutAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"

	result, err := client.ProTxUpdateRegistrar(proTxHash, operatorKey, votingAddress, payoutAddress, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

func TestProTxRegister(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	collateralHash := "5518ad303d6df237dd6cc0c8589dcf9ab286e6f7407a29aa072056ace1fbc47a"
	collateralIndex := 1
	ipAndPort := "144.202.120.18:19999"
	ownerAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorPubKey := "1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"
	votingAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorReward := 10.01
	payoutAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"
	feeSourceAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"
	submit := false

	result, err := client.ProTxRegister(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress, submit)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

func TestProTxRegisterFund(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	collateralAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	ipAndPort := "144.202.120.18:19999"
	ownerAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorPubKey := "1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"
	votingAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorReward := 10.01
	payoutAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"
	fundAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	submit := false

	result, err := client.ProTxRegisterFund(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, fundAddress, submit)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

func TestProTxRegisterPrepare(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	collateralHash := "5518ad303d6df237dd6cc0c8589dcf9ab286e6f7407a29aa072056ace1fbc47a"
	collateralIndex := 1
	ipAndPort := "144.202.120.18:19999"
	ownerAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorPubKey := "1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"
	votingAddress := "ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH"
	operatorReward := 10.01
	payoutAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"
	feeSourceAddress := "yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"

	result, err := client.ProTxRegisterPrepare(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress)
	if err != nil {
		t.Fatal(err)
	}

	modify := func(i interface{}) {
		r, ok := i.(*btcjson.ProTxRegisterPrepareResult)
		if !ok {
			t.Fatalf("cleanup function could not type cast from %T", i)
		}
		r.SignMessage = ""
		r.Tx = ""
	}

	var cli btcjson.ProTxInfoResult
	modifyThenCompareWithCliCommand(t, modify, result, &cli, "protx", "register_prepare", collateralHash, strconv.Itoa(collateralIndex), ipAndPort, ownerAddress, operatorPubKey, votingAddress, fmt.Sprint(operatorReward), payoutAddress, feeSourceAddress)
}

func TestProTxRegisterSubmit(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	tx := "0300010001a6970dbf500321a694fb5afb6c2f5269d4dfafe0b0bf70ed4639853de49fe87c0100000000feffffff01f109a503030000001976a9142621b120541dab04072b293f275d7213c4ffb9df88ac00000000d10100000000007ac4fbe1ac562007aa297a40f7e686b29acf9d58c8c06cdd37f26d3d30ad18550100000000000000000000000000ffff62ca58af4e200c1f2a18885154b6abd1b0736428e47ccddfa743084ceaabfe23865823aa696258245d8f94144fc33fb558528cd1742ef8f033d7b8c701d19cd6a561522c9e8d82bf72831f130b13c2258dd4b2ae2eb77b62a036be7be2a600001976a9142621b120541dab04072b293f275d7213c4ffb9df88ac90f10f99e0a72b561d3f83f0c1294cc0b88931444988eb7e7893fcb92f043f1c00"
	sig := "IFhavtOIwYRlqNw3AC8ODH7fVtZYMnH7VSPHcI7ZOu+YIWN/H430/TAvh7Es6hGFdwYBytkh9oQWuuYhPjTBXxE="

	result, err := client.ProTxRegisterSubmit(tx, sig)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

func TestProTxRevoke(t *testing.T) {
	t.Skip("wallet method is disabled because no wallet is loaded")
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"
	operatorPrivateKey := "-1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"

	result, err := client.ProTxRevoke(proTxHash, operatorPrivateKey, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

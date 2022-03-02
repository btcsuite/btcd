package rpcclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

	blsResult := btcjson.BLSResult{Secret: "secret", Public: "public"}
	client.httpClient.Transport = mockRoundTripperFunc(
		blsResult,
		expectBody(`{"jsonrpc":"1.0","method":"bls","params":["generate"],"id":1}`),
	)
	gen, err := client.BLSGenerate()
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = mockRoundTripperFunc(
		blsResult,
		expectBody(`{"jsonrpc":"1.0","method":"bls","params":["fromsecret","secret"],"id":2}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumListResult{},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["list"],"id":1}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumListResult{
			Llmq400_60: []string{"000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9"},
		},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["list"],"id":1}`),
	)

	list, err := client.QuorumList()
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Llmq400_60) == 0 {
		t.Fatal("list llmq_400_60 empty")
	}
	quorumHash := list.Llmq400_60[0]

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumInfoResult{
			Height:          264072,
			Type:            "llmq400_60",
			QuorumHash:      "000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",
			MinedBlock:      "000006113a77b35a0ed606b08ecb8e37f1ac7e2d773c365bd07064a72ae9a61d",
			QuorumPublicKey: "0644ff153b9b92c6a59e2adf4ef0b9836f7f6af05fe432ffdcb69bc9e300a2a70af4a8d9fc61323f6b81074d740033d2",
			SecretKeyShare:  "3da0d8f532309660f7f44aa0ed42c1569773b39c70f5771ce5604be77e50759e",
		},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["info",2,"000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",false],"id":2}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumSelectQuorumResult{
			QuorumHash: "000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",
		},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["selectquorum",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"],"id":1}`),
	)
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

	testCases := []struct {
		detailLevel btcjson.DetailLevel
		resp        interface{}
		req         string
	}{
		{
			detailLevel: btcjson.DetailLevelCounts,
			resp:        btcjson.QuorumDKGStatusCountsResult{},
			req:         `{"jsonrpc":"1.0","method":"quorum","params":["dkgstatus",0],"id":1}`,
		},
		{
			detailLevel: btcjson.DetailLevelIndexes,
			resp:        btcjson.QuorumDKGStatusIndexesResult{},
			req:         `{"jsonrpc":"1.0","method":"quorum","params":["dkgstatus",1],"id":2}`,
		},
		{
			detailLevel: btcjson.DetailLevelMembersProTxHashes,
			resp:        btcjson.QuorumDKGStatusMembersProTxHashesResult{},
			req:         `{"jsonrpc":"1.0","method":"quorum","params":["dkgstatus",2],"id":3}`,
		},
	}
	for _, tc := range testCases {
		dl := tc.detailLevel
		t.Run(fmt.Sprint(dl), func(t *testing.T) {
			client.httpClient.Transport = mockRoundTripperFunc(&tc.resp, expectBody(tc.req))

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

	client.httpClient.Transport = mockRoundTripperFunc(
		[]btcjson.QuorumMemberOfResult{},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["memberof","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"],"id":1}`),
	)
	result, err := client.QuorumMemberOf(proTxHash, 0)
	if err != nil {
		t.Fatal(err)
	}

	var cli []btcjson.QuorumMemberOfResult
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
	proTxHash := "ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"
	submit := false

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	client.httpClient.Transport = mockRoundTripperFunc(
		[]btcjson.QuorumMemberOfResult{
			{
				Height:          264072,
				Type:            "llmq_400_60",
				QuorumHash:      "000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",
				MinedBlock:      "000006113a77b35a0ed606b08ecb8e37f1ac7e2d773c365bd07064a72ae9a61d",
				QuorumPublicKey: "0644ff153b9b92c6a59e2adf4ef0b9836f7f6af05fe432ffdcb69bc9e300a2a70af4a8d9fc61323f6b81074d740033d2",
				IsValidMember:   false,
				MemberIndex:     10,
			},
		},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["memberof","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"],"id":1}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumSignResultWithBool{Result: true},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["sign",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234","51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239","000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",false],"id":2}`),
	)
	result, err := client.QuorumSign(quorumType, requestID, messageHash, quorumHash, submit)
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.QuorumSignResultWithBool{}
	compareWithCliCommand(t, result, cli, "quorum", "sign", fmt.Sprint(quorumType), requestID, messageHash, quorumHash, strconv.FormatBool(submit))

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.QuorumSignResultWithBool{Result: true},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["sign",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234","51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239","000004bfc56646880bfeb80a0b89ad955e557ead7b0f09bcc61e56c8473eaea9",true],"id":3}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		[]btcjson.QuorumSignResult{},
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["getrecsig",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234","51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"],"id":1}`),
	)
	result, err := client.QuorumGetRecSig(quorumType, requestID, messageHash)
	if err != nil {
		t.Fatal(err)
	}

	var cli []btcjson.QuorumSignResult
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

	resp := true
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["hasrecsig",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234","51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"],"id":1}`),
	)
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

	resp := true
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"quorum","params":["isconflicting",2,"abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234","51c11d287dfa85aef3eebb5420834c8e443e01d15c0b0a8e397d67e2e51aa239"],"id":1}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		&btcjson.ProTxInfoResult{ProTxHash: proTxHash},
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["info","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56"],"id":1}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		[]btcjson.ProTxInfoResult{},
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["list"],"id":1}`),
	)
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
	client.httpClient.Transport = mockRoundTripperFunc(
		[]btcjson.ProTxInfoResult{},
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["list","valid",true,415243],"id":2}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		btcjson.ProTxDiffResult{},
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["diff",75000,76000],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["update_service","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56","144.202.120.18:19999","084ceaabfe23865823aa696258245d8f94144fc33fb558528cd1742ef8f033d7b8c701d19cd6a561522c9e8d82bf7283"],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["update_registrar","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56","1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52","yP9kcJKUXt8BSfL8Pcj83fsMjBGhDAmBb6","yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["register","5518ad303d6df237dd6cc0c8589dcf9ab286e6f7407a29aa072056ace1fbc47a",1,"144.202.120.18:19999","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH","1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH",10.01,"yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ","yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ",false],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["register_fund","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH","144.202.120.18:19999","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH","1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH",10.01,"yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH",false],"id":1}`),
	)
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

	client.httpClient.Transport = mockRoundTripperFunc(
		&btcjson.ProTxRegisterPrepareResult{},
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["register_prepare","5518ad303d6df237dd6cc0c8589dcf9ab286e6f7407a29aa072056ace1fbc47a",1,"144.202.120.18:19999","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH","1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52","ydvWAgi23gDEfufbWjGDQbtDqTNrL4vpYH",10.01,"yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ","yPo4tSsQ2Y9RXK4io6mBke1RMr256VcLRQ"],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["register_submit","0300010001a6970dbf500321a694fb5afb6c2f5269d4dfafe0b0bf70ed4639853de49fe87c0100000000feffffff01f109a503030000001976a9142621b120541dab04072b293f275d7213c4ffb9df88ac00000000d10100000000007ac4fbe1ac562007aa297a40f7e686b29acf9d58c8c06cdd37f26d3d30ad18550100000000000000000000000000ffff62ca58af4e200c1f2a18885154b6abd1b0736428e47ccddfa743084ceaabfe23865823aa696258245d8f94144fc33fb558528cd1742ef8f033d7b8c701d19cd6a561522c9e8d82bf72831f130b13c2258dd4b2ae2eb77b62a036be7be2a600001976a9142621b120541dab04072b293f275d7213c4ffb9df88ac90f10f99e0a72b561d3f83f0c1294cc0b88931444988eb7e7893fcb92f043f1c00","IFhavtOIwYRlqNw3AC8ODH7fVtZYMnH7VSPHcI7ZOu+YIWN/H430/TAvh7Es6hGFdwYBytkh9oQWuuYhPjTBXxE="],"id":1}`),
	)
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

	resp := "1234567890123456789012345678901234567890123456789012345678901234"
	client.httpClient.Transport = mockRoundTripperFunc(
		&resp,
		expectBody(`{"jsonrpc":"1.0","method":"protx","params":["revoke","ec21749595a34d868cc366c0feefbd1cfaeb659c6acbc1e2e96fd1e714affa56","-1dce6ba29cc4c44acb188e030d86ded7ede3dbd828b32e12e21f30d62670ac52"],"id":1}`),
	)
	result, err := client.ProTxRevoke(proTxHash, operatorPrivateKey, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 64 {
		t.Fatal("returned key was not 64 characters: ", result)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip ...
func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func readBody(r *http.Request) ([]byte, error) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("unable read a body content from a request: %w", err)
	}
	_ = r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	return data, nil
}

func expectBody(expected string) func(r *http.Request) error {
	return func(r *http.Request) error {
		data, err := readBody(r)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, []byte(expected)) {
			return fmt.Errorf("a body content is not equal, expected: %s, actual: %s", expected, data)
		}
		return nil
	}
}

func mockRoundTripperFunc(resp interface{}, expects ...func(r *http.Request) error) roundTripperFunc {
	return func(r *http.Request) (*http.Response, error) {
		err := r.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, expect := range expects {
			err = expect(r)
			if err != nil {
				return nil, err
			}
		}
		raw, err := json.Marshal(&resp)
		if err != nil {
			return nil, err
		}
		rawResp := rawResponse{
			Result: raw,
		}
		raw, err = json.Marshal(&rawResp)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer(raw)),
		}, nil
	}
}

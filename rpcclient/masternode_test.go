package rpcclient

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"reflect"
	"testing"

	"github.com/dashevo/dashd-go/btcjson"
)

func TestGetBlockCount(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	blockCount, err := client.GetBlockCount()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get("https://testnet-insight.dashevo.org/insight-api/status")
	if err != nil {
		t.Fatal("colud not get insite", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("colud not read body", err)
	}

	r := struct {
		Info struct {
			Blocks int64 `json:"blocks"`
		} `json:"info"`
	}{}
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal("could not unmarshal body", err)
	}
	if bl := r.Info.Blocks; bl != blockCount {
		t.Error("node not synced with blockchain, block count did not match insight", blockCount, bl)
	}
	t.Log("Block count:", blockCount)
}

func TestMasternodeStatus(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeStatus()
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.MasternodeStatusResult{}
	compareWithCliCommand(t, result, cli, "masternode", "status")
}

func TestMasternodeCount(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeCount()
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.MasternodeCountResult{}
	compareWithCliCommand(t, result, cli, "masternode", "count")
}

func TestMasternodeCurrent(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeCurrent()
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.MasternodeResult{}
	compareWithCliCommand(t, result, cli, "masternode", "current")
}

func TestMasternodeOutputs(t *testing.T) {
	t.Skip("error: -32601: Method not found (wallet method is disabled because no wallet is loaded)")

	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeOutputs()
	if err != nil {
		t.Fatal(err)
	}

	cli := &map[string]string{}
	compareWithCliCommand(t, &result, cli, "masternode", "outputs")
}

func TestMasternodeWinner(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeWinner()
	if err != nil {
		t.Fatal(err)
	}

	cli := &btcjson.MasternodeResult{}
	compareWithCliCommand(t, result, cli, "masternode", "winner")
}

func TestMasternodeWinners(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	t.Run("no params", func(t *testing.T) {
		result, err := client.MasternodeWinners(0, "")
		if err != nil {
			t.Fatal(err)
		}
		cli := &map[string]string{}
		compareWithCliCommand(t, &result, cli, "masternode", "winners")
	})

	t.Run("just count", func(t *testing.T) {
		result, err := client.MasternodeWinners(20, "")
		if err != nil {
			t.Fatal(err)
		}
		cli := &map[string]string{}
		compareWithCliCommand(t, &result, cli, "masternode", "winners", "20")
	})

	t.Run("count and filter", func(t *testing.T) {
		result, err := client.MasternodeWinners(30, "yP8A3")
		if err != nil {
			t.Fatal(err)
		}
		cli := &map[string]string{}
		compareWithCliCommand(t, &result, cli, "masternode", "winners", "30", "yP8A3")
	})
}

func TestMasternodeList(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.MasternodeList("addr", "")
	if err != nil {
		t.Fatal(err)
	}

	cli := &map[string]string{}
	compareWithCliCommand(t, result, cli, "masternodelist", "addr")

	resultJSON, err := client.MasternodeListJSON("")
	if err != nil {
		t.Fatal(err)
	}

	cliJSON := &map[string]btcjson.MasternodelistResultJSON{}
	compareWithCliCommand(t, &resultJSON, cliJSON, "masternodelist", "json")
}

func compareWithCliCommand(t *testing.T, rpc, cli interface{}, cmds ...string) {
	modifyThenCompareWithCliCommand(t, nil, rpc, cli, cmds...)
}

func modifyThenCompareWithCliCommand(t *testing.T, modify func(interface{}), rpc, cli interface{}, cmds ...string) {
	cmd := append([]string{"-testnet"}, cmds...)
	out, err := exec.Command("dash-cli", cmd...).Output()
	if err != nil {
		t.Fatal("Could not run dash-cli command", err)
	}

	if err := json.Unmarshal(out, &cli); err != nil {
		t.Log(string(out))
		t.Fatal("Could not marshal dash-cli output", err)
	}

	if modify != nil {
		modify(rpc)
		modify(cli)
	}

	if !reflect.DeepEqual(rpc, cli) {
		t.Error("rpc result did not match cli")
		t.Logf("rpc result: %#v", rpc)
		t.Logf("cli result: %#v", cli)
		t.Logf("cli string: %s", string(out))
	}
}

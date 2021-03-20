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

func TestMasternodelist(t *testing.T) {
	client, err := New(connCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown()

	result, err := client.Masternodelist("addr", "")
	if err != nil {
		t.Fatal(err)
	}

	cli := &map[string]string{}
	compareWithCliCommand(t, result, cli, "masternodelist", "addr")

	resultJSON, err := client.MasternodelistJSON("")
	if err != nil {
		t.Fatal(err)
	}

	cliJSON := &map[string]btcjson.MasternodelistResultJSON{}
	compareWithCliCommand(t, &resultJSON, cliJSON, "masternodelist", "json")
}

func compareWithCliCommand(t *testing.T, rpc, cli interface{}, cmds ...string) {
	cmd := append([]string{"-testnet"}, cmds...)
	out, err := exec.Command("dash-cli", cmd...).Output()
	if err != nil {
		t.Fatal("Could not run dash-cli command", err)
	}

	if err := json.Unmarshal(out, &cli); err != nil {
		t.Log(string(out))
		t.Fatal("Could not marshal dash-cli output", err)
	}

	if !reflect.DeepEqual(rpc, cli) {
		t.Error("rpc result did not match cli")
		t.Logf("rpc result: %#v", rpc)
		t.Logf("cli result: %#v", cli)
		t.Logf("cli string: %s", string(out))
	}
}

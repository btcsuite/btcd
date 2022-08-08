// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lbryio/lbcd/rpcclient"
	"github.com/lbryio/lbcd/wire"
	"github.com/lbryio/lbcutil"
)

var (
	coinid      = flag.String("coinid", "1425", "Coin ID")
	stratum     = flag.String("stratum", "", "Stratum server")
	stratumPass = flag.String("stratumpass", "", "Stratum server password")
	rpcserver   = flag.String("rpcserver", "localhost:9245", "LBCD RPC server")
	rpcuser     = flag.String("rpcuser", "rpcuser", "LBCD RPC username")
	rpcpass     = flag.String("rpcpass", "rpcpass", "LBCD RPC password")
	notls       = flag.Bool("notls", false, "Connect to LBCD with TLS disabled")
	run         = flag.String("run", "", "Run custom shell command")
)

func onFilteredBlockConnected(height int32, header *wire.BlockHeader, txns []*lbcutil.Tx) {

	blockHash := header.BlockHash().String()

	log.Printf("Block connected: %v (%d) %v", blockHash, height, header.Timestamp)

	if cmd := *run; len(cmd) != 0 {
		cmd = strings.ReplaceAll(cmd, "%s", blockHash)
		err := execCustomCommand(cmd)
		if err != nil {
			log.Printf("ERROR: execCustomCommand: %s", err)
		}
	}

	if len(*stratum) > 0 && len(*stratumPass) > 0 {
		err := stratumUpdateBlock(*stratum, *stratumPass, *coinid, blockHash)
		if err != nil {
			log.Printf("ERROR: stratumUpdateBlock: %s", err)
		}
	}
}
func execCustomCommand(cmd string) error {
	strs := strings.Split(cmd, " ")
	path, err := exec.LookPath(strs[0])
	if errors.Is(err, exec.ErrDot) {
		err = nil
	}
	if err != nil {
		return err
	}
	c := exec.Command(path, strs[1:]...)
	c.Stdout = os.Stdout
	return c.Run()
}

func stratumUpdateBlock(stratum, stratumPass, coinid, blockHash string) error {
	addr, err := net.ResolveTCPAddr("tcp", stratum)
	if err != nil {
		return fmt.Errorf("can't resolve addr: %w", err)
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return fmt.Errorf("can't dial tcp: %w", err)
	}
	defer conn.Close()

	msg := fmt.Sprintf(`{"id":1,"method":"mining.update_block","params":[%q,%s,%q]}`,
		stratumPass, coinid, blockHash)

	_, err = conn.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("can't write message: %w", err)
	}

	return nil
}
func main() {

	flag.Parse()

	ntfnHandlers := rpcclient.NotificationHandlers{
		OnFilteredBlockConnected: onFilteredBlockConnected,
	}

	// Connect to local lbcd RPC server using websockets.
	lbcdHomeDir := lbcutil.AppDataDir("lbcd", false)
	certs, err := ioutil.ReadFile(filepath.Join(lbcdHomeDir, "rpc.cert"))
	if err != nil {
		log.Fatalf("can't read lbcd certificate: %s", err)
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         *rpcserver,
		Endpoint:     "ws",
		User:         *rpcuser,
		Pass:         *rpcpass,
		Certificates: certs,
		DisableTLS:   *notls,
	}
	client, err := rpcclient.New(connCfg, &ntfnHandlers)
	if err != nil {
		log.Fatalf("can't create rpc client: %s", err)
	}

	// Register for block connect and disconnect notifications.
	if err = client.NotifyBlocks(); err != nil {
		log.Fatalf("can't register block notification: %s", err)
	}
	log.Printf("NotifyBlocks: Registration Complete")

	// Get the current block count.
	blockCount, err := client.GetBlockCount()
	if err != nil {
		log.Fatalf("can't get block count: %s", err)
	}
	log.Printf("Block count: %d", blockCount)

	// Wait until the client either shuts down gracefully (or the user
	// terminates the process with Ctrl+C).
	client.WaitForShutdown()
}

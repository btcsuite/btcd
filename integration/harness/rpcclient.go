// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

import (
	"fmt"
	"math"
	"time"

	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/rpcclient"
)

// RPCConnection is a helper class wrapping rpcclient.Client for test calls
type RPCConnection struct {
	rpcClient      *rpcclient.Client
	MaxConnRetries int
	isConnected    bool
}

// NewRPCConnection produces new instance of the RPCConnection
func NewRPCConnection(config *rpcclient.ConnConfig, maxConnRetries int, ntfnHandlers *rpcclient.NotificationHandlers) *rpcclient.Client {
	var client *rpcclient.Client
	var err error

	for i := 0; i < maxConnRetries; i++ {
		client, err = rpcclient.New(config, ntfnHandlers)
		if err != nil {
			fmt.Println("err: " + err.Error())
			time.Sleep(time.Duration(math.Log(float64(i+3))) * 50 * time.Millisecond)
			continue
		}
		break
	}
	if client == nil {
		integration.ReportTestSetupMalfunction(fmt.Errorf("client connection timedout"))
	}
	return client
}

// Connect switches RPCConnection into connected state establishing RPCConnection to the rpcConf target
func (client *RPCConnection) Connect(rpcConf *rpcclient.ConnConfig, ntfnHandlers *rpcclient.NotificationHandlers) {
	if client.isConnected {
		integration.ReportTestSetupMalfunction(fmt.Errorf("%v is already connected", client.rpcClient))
	}
	client.isConnected = true
	rpcClient := NewRPCConnection(rpcConf, client.MaxConnRetries, ntfnHandlers)
	err := rpcClient.NotifyBlocks()
	integration.CheckTestSetupMalfunction(err)
	client.rpcClient = rpcClient
}

// Disconnect switches RPCConnection into offline state
func (client *RPCConnection) Disconnect() {
	if !client.isConnected {
		integration.ReportTestSetupMalfunction(fmt.Errorf("%v is already disconnected", client))
	}
	client.isConnected = false
	client.rpcClient.Disconnect()
	client.rpcClient.Shutdown()
}

// IsConnected flags RPCConnection state
func (client *RPCConnection) IsConnected() bool {
	return client.isConnected
}

// Connection returns rpcclient.Client for API calls
func (client *RPCConnection) Connection() *rpcclient.Client {
	return client.rpcClient
}

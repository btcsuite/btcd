// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/commandline"
	"github.com/btcsuite/btcd/integration/gobuilder"
	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/integration/harness/memwallet"
	"github.com/btcsuite/btcd/integration/harness/testnode"
)

// Default harness name
const mainHarnessName = "main"

// SimpleTestSetup harbours:
// - rpctest setup
// - csf-fork test setup
// - and bip0009 test setup
type SimpleTestSetup struct {
	// harnessPool stores and manages harnesses
	// multiple harness instances may be run concurrently, to allow for testing
	// complex scenarios involving multiple nodes.
	harnessPool *integration.Pool

	// Regnet25 creates a regnet test harness
	// with 25 mature outputs.
	Regnet25 *ChainWithMatureOutputsSpawner

	// Regnet5 creates a regnet test harness
	// with 5 mature outputs.
	Regnet5 *ChainWithMatureOutputsSpawner

	// Regnet1 creates a regnet test harness
	// with 1 mature output.
	Regnet1 *ChainWithMatureOutputsSpawner

	// Simnet1 creates a simnet test harness
	// with 1 mature output.
	Simnet1 *ChainWithMatureOutputsSpawner

	// Regnet0 creates a regnet test harness
	// with only the genesis block.
	Regnet0 *ChainWithMatureOutputsSpawner

	// Simnet0 creates a simnet test harness
	// with only the genesis block.
	Simnet0 *ChainWithMatureOutputsSpawner

	// NodeFactory produces a new TestNode instance upon request
	NodeFactory harness.TestNodeFactory

	// WalletFactory produces a new TestWallet instance upon request
	WalletFactory harness.TestWalletFactory

	// WorkingDir defines test setup working dir
	WorkingDir *integration.TempDirHandler

	// nodeGoBuilder builds test node Go-code for the tests
	nodeGoBuilder *gobuilder.GoBuider
}

// TearDown all harnesses in test Pool.
// This includes removing all temporary directories,
// and shutting down any created processes.
func (setup *SimpleTestSetup) TearDown() {
	setup.harnessPool.DisposeAll()
	setup.nodeGoBuilder.Dispose()
	setup.WorkingDir.Dispose()
}

// Setup deploys this test setup
func Setup() *SimpleTestSetup {
	setup := &SimpleTestSetup{
		WalletFactory: &memwallet.WalletFactory{},
		//Network:       &chaincfg.RegressionNetParams,
		WorkingDir: integration.NewTempDir(setupWorkingDir(), "simpleregtest").MakeDir(),
	}

	setup.nodeGoBuilder = setupBuild("btcd", setup.WorkingDir.Path())
	setup.NodeFactory = &testnode.NodeFactory{
		NodeExecutablePathProvider: setup.nodeGoBuilder,
	}
	setup.nodeGoBuilder.Build()

	portManager := &LazyPortManager{
		BasePort: 20000,
		offset:   0,
	}

	// Deploy harness spawner with generated
	// test chain of 25 mature outputs
	setup.Regnet25 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   true,
		DebugWalletOutput: true,
		NumMatureOutputs:  25,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.RegressionNetParams,
	}

	// Deploy harness spawner with generated
	// test chain of 5 mature outputs
	setup.Regnet5 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   true,
		DebugWalletOutput: true,
		NumMatureOutputs:  5,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.RegressionNetParams,
	}

	setup.Regnet1 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   true,
		DebugWalletOutput: true,
		NumMatureOutputs:  1,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.RegressionNetParams,
		NodeStartExtraArguments: map[string]interface{}{
			"rejectnonstd": commandline.NoArgumentValue,
		},
	}

	setup.Simnet1 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   true,
		DebugWalletOutput: true,
		NumMatureOutputs:  1,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.SimNetParams,
		NodeStartExtraArguments: map[string]interface{}{
			"rejectnonstd": commandline.NoArgumentValue,
		},
	}

	// Deploy harness spawner with empty test chain
	setup.Regnet0 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   false,
		DebugWalletOutput: false,
		NumMatureOutputs:  0,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.RegressionNetParams,
	}
	// Deploy harness spawner with empty test chain
	setup.Simnet0 = &ChainWithMatureOutputsSpawner{
		WorkingDir:        setup.WorkingDir.Path(),
		DebugNodeOutput:   false,
		DebugWalletOutput: false,
		NumMatureOutputs:  0,
		NetPortManager:    portManager,
		WalletFactory:     setup.WalletFactory,
		NodeFactory:       setup.NodeFactory,
		ActiveNet:         &chaincfg.SimNetParams,
	}

	setup.harnessPool = integration.NewPool(setup.Regnet25)

	return setup
}

func setupWorkingDir() string {
	testWorkingDir, err := ioutil.TempDir("", "integrationtest")
	if err != nil {
		fmt.Println("Unable to create working dir: ", err)
		os.Exit(-1)
	}
	return testWorkingDir
}

func setupBuild(buildName string, workingDir string) *gobuilder.GoBuider {
	nodeProjectGoPath := gobuilder.DetermineProjectPackagePath(buildName)
	tempBinDir := filepath.Join(workingDir, "bin")
	integration.MakeDirs(tempBinDir)

	nodeGoBuilder := &gobuilder.GoBuider{
		GoProjectPath:    nodeProjectGoPath,
		OutputFolderPath: tempBinDir,
		BuildFileName:    buildName,
	}
	return nodeGoBuilder
}

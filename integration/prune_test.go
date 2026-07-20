// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func TestPrune(t *testing.T) {
	t.Parallel()

	r, err := rpctest.New()
	require.NoError(t, err)

	// Setup a pruned node.
	err = r.SetUp(rpctest.SOpts{Args: []string{"--prune=1536"}})
	if err != nil {
		require.NoError(t, err)
	}
	t.Cleanup(func() { r.TearDown() })

	// Check that the rpc call for block chain info comes back correctly.
	chainInfo, err := r.Client.GetBlockChainInfo()
	require.NoError(t, err)

	if !chainInfo.Pruned {
		t.Fatalf("expected the node to be pruned but the pruned "+
			"boolean was %v", chainInfo.Pruned)
	}
}

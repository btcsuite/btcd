// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

var (
	ourVersion = uint32(3)

	pedro = chaincfg.Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []chaincfg.Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsIgnore:    true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsIgnore:    false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsIgnore:    false,
				IsNo:        true,
			},
		},
	}
)

func defaultParams() *chaincfg.Params {
	params := &chaincfg.SimNetParams
	params.Deployments = make(map[uint32][]chaincfg.ConsensusDeployment)
	params.Deployments[ourVersion] = []chaincfg.ConsensusDeployment{
		{
			Vote:       pedro,
			StartTime:  uint64(time.Now().Unix()),                       // we need to futz with this
			ExpireTime: uint64(time.Now().Add(10 * time.Second).Unix()), // we need to futz with this
		},
	}

	return params
}

func TestSerializeDeserialize(t *testing.T) {
	params := defaultParams()
	ourDeployment := &params.Deployments[ourVersion][0]
	blob := serializeDeploymentCacheParams(ourDeployment)
	//t.Logf("blob: %x", blob)

	deserialized, err := deserializeDeploymentCacheParams(blob)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if deserialized.Vote.Mask != pedro.Mask {
		t.Fatalf("invalid Mask")
	}
	if deserialized.StartTime != ourDeployment.StartTime {
		t.Fatalf("invalid StartTime")
	}
	if deserialized.ExpireTime != ourDeployment.ExpireTime {
		t.Fatalf("invalid ExpireTime")
	}
	if len(deserialized.Vote.Choices) != len(ourDeployment.Vote.Choices) {
		t.Fatalf("invalid len deserialized.Vote.Choices got "+
			"%v expected %v", len(deserialized.Vote.Choices),
			len(ourDeployment.Vote.Choices))
	}
	for i := 0; i < len(deserialized.Vote.Choices); i++ {
		if deserialized.Vote.Choices[i].Bits !=
			ourDeployment.Vote.Choices[i].Bits {
			t.Fatalf("invalid Bits %v got %v expected %v", i,
				deserialized.Vote.Choices[i].Bits,
				ourDeployment.Vote.Choices[i].Bits)
		}
		if deserialized.Vote.Choices[i].IsIgnore !=
			ourDeployment.Vote.Choices[i].IsIgnore {
			t.Fatalf("invalid IsIgnore %v got %v expected %v", i,
				deserialized.Vote.Choices[i].IsIgnore,
				ourDeployment.Vote.Choices[i].IsIgnore)
		}
		if deserialized.Vote.Choices[i].IsNo !=
			ourDeployment.Vote.Choices[i].IsNo {
			t.Fatalf("invalid IsNo %v got %v expected %v", i,
				deserialized.Vote.Choices[i].IsNo,
				ourDeployment.Vote.Choices[i].IsNo)
		}
	}
}

func TestVoting(t *testing.T) {
	params := defaultParams()

	tests := []struct {
		name              string
		numNodes          uint32
		vote              chaincfg.Vote
		blockVersion      int32
		startStakeVersion uint32
		voteBits          uint16
		expectedState     []ThresholdState
	}{
		{
			name:              "pedro 100% yes",
			numNodes:          params.MinerConfirmationWindow * 4,
			vote:              pedro,
			blockVersion:      3,
			startStakeVersion: ourVersion,
			voteBits:          0x2,
			expectedState:     []ThresholdState{ThresholdActive},
		},
		{
			name:              "pedro 100% no",
			numNodes:          params.MinerConfirmationWindow * 4,
			vote:              pedro,
			blockVersion:      3,
			startStakeVersion: ourVersion,
			voteBits:          0x4,
			expectedState:     []ThresholdState{ThresholdFailed},
		},
		{
			name:              "pedro 100% abstain",
			numNodes:          params.MinerConfirmationWindow * 4,
			vote:              pedro,
			blockVersion:      3,
			startStakeVersion: ourVersion,
			voteBits:          0x0,
			expectedState:     []ThresholdState{ThresholdStarted},
		},
	}

	for _, test := range tests {
		// We have to reset the cache for every test.
		bc := &BlockChain{
			chainParams:      params,
			deploymentCaches: newThresholdCaches(ourVersion),
		}
		genesisNode := genesisBlockNode(params)
		genesisNode.header.StakeVersion = test.startStakeVersion

		t.Logf("running: %v\n", test.name)

		var currentNode *blockNode
		currentNode = genesisNode
		for k := range test.expectedState {
			for i := uint32(1); i <= test.numNodes; i++ {
				// Make up a header.
				header := &wire.BlockHeader{
					Version:      test.blockVersion,
					Height:       uint32(i),
					Nonce:        uint32(0),
					StakeVersion: test.startStakeVersion,
					Timestamp:    time.Now(),
				}
				node := newBlockNode(header, &chainhash.Hash{}, 0,
					[]chainhash.Hash{}, []chainhash.Hash{},
					[]uint32{}, []uint16{})
				node.height = int64(i)
				node.parent = currentNode

				// set stake versions and vote bits
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					node.voterVersions = append(node.voterVersions, test.startStakeVersion)
					node.voteBits = append(node.voteBits, test.voteBits)
				}

				currentNode = node
				bc.bestNode = currentNode
			}
			ts, err := bc.ThresholdState(ourVersion, pedro.Id)
			if err != nil {
				t.Fatalf("ThresholdState: %v", err)
			}
			if ts != test.expectedState[k] {
				t.Fatalf("%v got state %v wanted %v", test.name, ts,
					test.expectedState[k])
			}
		}
	}
}

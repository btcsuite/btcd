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
	posVersion = uint32(4)
	powVersion = int32(4)

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

func defaultParams() chaincfg.Params {
	params := chaincfg.SimNetParams
	params.Deployments = make(map[uint32][]chaincfg.ConsensusDeployment)
	params.Deployments[posVersion] = []chaincfg.ConsensusDeployment{{
		Vote: pedro,
		StartTime: uint64(time.Now().Add(time.Duration(
			params.RuleChangeActivationInterval) *
			time.Second).Unix()),
		ExpireTime: uint64(time.Now().Add(24 * time.Hour).Unix()),
	}}

	return params
}

func TestSerializeDeserialize(t *testing.T) {
	params := defaultParams()
	ourDeployment := &params.Deployments[posVersion][0]
	blob := serializeDeploymentCacheParams(ourDeployment)

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

func TestNoQuorum(t *testing.T) {
	params := defaultParams()
	bc := &BlockChain{
		chainParams:      &params,
		deploymentCaches: newThresholdCaches(&params),
		index:            make(map[chainhash.Hash]*blockNode),
	}
	genesisNode := genesisBlockNode(&params)
	genesisNode.header.StakeVersion = posVersion

	var currentNode *blockNode
	currentNode = genesisNode
	currentTimestamp := time.Now()
	currentHeight := uint32(1)

	// get to svi
	for i := uint32(0); i < uint32(params.StakeValidationHeight); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}
	t.Logf("Height %v", currentNode.height)
	ts, err := bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(SVI): %v", err)
	}
	tse := ThresholdStateTuple{
		State:  ThresholdDefined,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to started
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval-1); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			node.votes = append(node.votes, VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01})
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(started): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to quorum - 1
	voteCount := uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			if voteCount < params.RuleChangeActivationQuorum-1 {
				v.Bits = 0x05 // vote no
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to exact quorum but with 75%-1 yes votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			// 119 yes, 41 no -> 120 == 75% and 120 reaches quorum
			quorum := params.RuleChangeActivationQuorum*
				params.RuleChangeActivationMultiplier/
				params.RuleChangeActivationDivisor - 1
			if voteCount < quorum {
				v.Bits = 0x05 // vote no
			} else {
				if voteCount < params.RuleChangeActivationQuorum {
					v.Bits = 0x03 // vote yes
				} else {
					v.Bits = 0x01 // ignore
				}
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to exact quorum with exactly 75% of votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			// 120 yes, 40 no -> 120 == 75% and 120 reaches quorum
			quorum := params.RuleChangeActivationQuorum *
				params.RuleChangeActivationMultiplier /
				params.RuleChangeActivationDivisor
			if voteCount < quorum {
				v.Bits = 0x05 // vote no
			} else {
				if voteCount < params.RuleChangeActivationQuorum {
					v.Bits = 0x03 // vote yes
				} else {
					v.Bits = 0x01 // ignore
				}
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdFailed,
		Choice: 0x02,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}
}

func TestYesQuorum(t *testing.T) {
	params := defaultParams()
	bc := &BlockChain{
		chainParams:      &params,
		deploymentCaches: newThresholdCaches(&params),
		index:            make(map[chainhash.Hash]*blockNode),
	}
	genesisNode := genesisBlockNode(&params)
	genesisNode.header.StakeVersion = posVersion

	var currentNode *blockNode
	currentNode = genesisNode
	currentTimestamp := time.Now()
	currentHeight := uint32(1)

	// get to svi
	for i := uint32(0); i < uint32(params.StakeValidationHeight); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}
	t.Logf("Height %v", currentNode.height)
	ts, err := bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(SVI): %v", err)
	}
	tse := ThresholdStateTuple{
		State:  ThresholdDefined,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to started
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval-1); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			node.votes = append(node.votes, VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01})
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(started): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to quorum - 1
	voteCount := uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			if voteCount < params.RuleChangeActivationQuorum-1 {
				v.Bits = 0x03 // vote yes
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to exact quorum but with 75%-1 yes votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			// 119 yes, 41 no -> 120 == 75% and 120 reaches quorum
			quorum := params.RuleChangeActivationQuorum*
				params.RuleChangeActivationMultiplier/
				params.RuleChangeActivationDivisor - 1
			if voteCount < quorum {
				v.Bits = 0x03 // vote yes
			} else {
				if voteCount < params.RuleChangeActivationQuorum {
					v.Bits = 0x05 // vote no
				} else {
					v.Bits = 0x01 // ignore
				}
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}

	// get to exact quorum with exactly 75% of votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Make up a header.
		header := &wire.BlockHeader{
			Version:      powVersion,
			Height:       currentHeight,
			Nonce:        uint32(0),
			StakeVersion: posVersion,
			Timestamp:    currentTimestamp,
		}
		hash := header.BlockHash()
		node := newBlockNode(header, &hash, 0,
			[]chainhash.Hash{}, []chainhash.Hash{},
			[]VoteVersionTuple{})
		node.height = int64(currentHeight)
		node.parent = currentNode

		// set stake versions and vote bits
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			v := VoteVersionTuple{
				Version: posVersion,
				Bits:    0x01,
			}
			// 120 yes, 40 no -> 120 == 75% and 120 reaches quorum
			quorum := params.RuleChangeActivationQuorum *
				params.RuleChangeActivationMultiplier /
				params.RuleChangeActivationDivisor
			if voteCount < quorum {
				v.Bits = 0x03 // vote yes
			} else {
				if voteCount < params.RuleChangeActivationQuorum {
					v.Bits = 0x05 // vote no
				} else {
					v.Bits = 0x01 // ignore
				}
			}
			node.votes = append(node.votes, v)
			voteCount++
		}

		currentNode = node
		bc.bestNode = currentNode

		// fake index
		bc.index[hash] = node

		currentHeight++
		currentTimestamp = currentTimestamp.Add(time.Second)
	}

	t.Logf("Height %v", currentNode.height)
	ts, err = bc.ThresholdState(&currentNode.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdLockedIn,
		Choice: 0x01,
	}
	if ts != tse {
		t.Fatalf("expected %v got %v", ts.Choice, tse.Choice)
	}
}

func TestVoting(t *testing.T) {
	params := defaultParams()

	type voteCount struct {
		vote  VoteVersionTuple
		count uint32
	}

	tests := []struct {
		name              string
		vote              chaincfg.Vote
		blockVersion      int32
		startStakeVersion uint32
		voteBitsCounts    []voteCount
		expectedState     []ThresholdStateTuple
	}{
		{
			name:              "pedro too shallow",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight) - 1,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
			},
		},
		{
			name:              "pedro greater PoS version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion - 1,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
			},
		},
		{
			name:              "pedro smaller PoS version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion + 1,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdLockedIn,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
			},
		},
		{
			name:              "pedro smaller PoW version",
			vote:              pedro,
			blockVersion:      powVersion - 1,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
			},
		},
		{
			name:              "pedro greater PoW version",
			vote:              pedro,
			blockVersion:      powVersion + 1,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdLockedIn,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
			},
		},
		{
			name:              "pedro 100% yes, wrong version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion + 1,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion + 1,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion + 1,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion + 1,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion + 1,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdLockedIn,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
			},
		},
		{
			name:              "pedro 100% yes",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x03},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdLockedIn,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
				{
					State:  ThresholdActive,
					Choice: 0x01,
				},
			},
		},
		{
			name:              "pedro 100% no",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x05},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x05},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdFailed,
					Choice: 0x02,
				},
				{
					State:  ThresholdFailed,
					Choice: 0x02,
				},
				{
					State:  ThresholdFailed,
					Choice: 0x02,
				},
			},
		},
		{
			name:              "pedro 100% abstain",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: []ThresholdStateTuple{
				{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
				{
					State:  ThresholdStarted,
					Choice: invalidChoice,
				},
			},
		},
		//{
		//	name:              "less than quorum",
		//	numNodes:          params.RuleChangeActivationInterval,
		//	vote:              pedro,
		//	blockVersion: powVersion,
		//	startStakeVersion: posVersion,
		//	voteBitsCounts:    []voteCount{{vote: 0x02, count: 100}},
		//	expectedState:     ThresholdStateTuple{ThresholdStarted, invalidChoice},
		//},
	}

	for _, test := range tests {
		// Reset params.
		params = defaultParams()
		// We have to reset the cache for every test.
		bc := &BlockChain{
			chainParams:      &params,
			deploymentCaches: newThresholdCaches(&params),
			index:            make(map[chainhash.Hash]*blockNode),
		}
		genesisNode := genesisBlockNode(&params)
		genesisNode.header.StakeVersion = test.startStakeVersion

		t.Logf("running: %v", test.name)

		var currentNode *blockNode
		currentNode = genesisNode
		currentTimestamp := time.Now()
		currentHeight := uint32(1)
		for k := range test.expectedState {
			for i := uint32(0); i < test.voteBitsCounts[k].count; i++ {
				// Make up a header.
				header := &wire.BlockHeader{
					Version:      test.blockVersion,
					Height:       currentHeight,
					Nonce:        uint32(0),
					StakeVersion: test.startStakeVersion,
					Timestamp:    currentTimestamp,
				}
				hash := header.BlockHash()
				node := newBlockNode(header, &hash, 0,
					[]chainhash.Hash{}, []chainhash.Hash{},
					[]VoteVersionTuple{})
				node.height = int64(currentHeight)
				node.parent = currentNode

				// set stake versions and vote bits
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					node.votes = append(node.votes,
						test.voteBitsCounts[k].vote)
				}

				currentNode = node
				bc.bestNode = currentNode

				// fake index
				bc.index[hash] = node

				currentHeight++
				currentTimestamp = currentTimestamp.Add(time.Second)
			}
			t.Logf("Height %v", currentNode.height)
			ts, err := bc.ThresholdState(&currentNode.hash,
				posVersion, pedro.Id)
			if err != nil {
				t.Fatalf("ThresholdState(%v): %v", k, err)
			}
			if ts != test.expectedState[k] {
				t.Fatalf("%v.%v (%v) got state %v wanted state"+
					" %v got choice %v wanted choice %v",
					test.name, test.vote.Id, k, ts,
					test.expectedState[k], ts.Choice,
					test.expectedState[k].Choice)
			}
		}
	}
}

// Parallel test.
const (
	testDummy1ID    = "testdummy1"
	vbTestDummy1No  = 0x02
	vbTestDummy1Yes = 0x04

	testDummy2ID    = "testdummy2"
	vbTestDummy2No  = 0x08
	vbTestDummy2Yes = 0x10
)

var (
	// testDummy1 is a voting agenda used throughout these tests.
	testDummy1 = chaincfg.Vote{
		Id:          testDummy1ID,
		Description: "",
		Mask:        0x6, // 0b0110
		Choices: []chaincfg.Choice{{
			Id:          "abstain",
			Description: "abstain voting for change",
			Bits:        0x0000,
			IsIgnore:    true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0002, // Bit 1
			IsIgnore:    false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0004, // Bit 2
			IsIgnore:    false,
			IsNo:        false,
		}},
	}

	// testDummy2 is a voting agenda used throughout these tests.
	testDummy2 = chaincfg.Vote{
		Id:          testDummy2ID,
		Description: "",
		Mask:        0x18, // 0b11000
		Choices: []chaincfg.Choice{{
			Id:          "abstain",
			Description: "abstain voting for change",
			Bits:        0x0000,
			IsIgnore:    true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0008, // Bit 3
			IsIgnore:    false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0010, // Bit 4
			IsIgnore:    false,
			IsNo:        false,
		}},
	}
)

func defaultParallelParams() chaincfg.Params {
	params := chaincfg.SimNetParams
	params.Deployments = make(map[uint32][]chaincfg.ConsensusDeployment)
	params.Deployments[posVersion] = []chaincfg.ConsensusDeployment{
		{
			Vote: testDummy1,
			StartTime: uint64(time.Now().Add(time.Duration(params.RuleChangeActivationInterval) *
				time.Second).Unix()),
			ExpireTime: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
		{
			Vote: testDummy2,
			StartTime: uint64(time.Now().Add(time.Duration(params.RuleChangeActivationInterval) *
				time.Second).Unix()),
			ExpireTime: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
	}

	return params
}

func TestVotingParallel(t *testing.T) {
	params := defaultParallelParams()

	type voteCount struct {
		vote  VoteVersionTuple
		count uint32
	}

	tests := []struct {
		name              string
		vote              []chaincfg.Vote
		blockVersion      int32
		startStakeVersion uint32
		voteBitsCounts    []voteCount
		expectedState     [][]ThresholdStateTuple
	}{
		{
			name:              "parallel",
			vote:              []chaincfg.Vote{testDummy1, testDummy2},
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: uint32(params.StakeValidationHeight),
				},
				{
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval - 1,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion - 1,
						Bits:    vbTestDummy1Yes | vbTestDummy2No},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    vbTestDummy1Yes | vbTestDummy2No},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				}, {
					vote: VoteVersionTuple{
						Version: posVersion,
						Bits:    0x01},
					count: params.RuleChangeActivationInterval,
				},
			},
			expectedState: [][]ThresholdStateTuple{
				// 0
				{
					{
						State:  ThresholdDefined,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdStarted,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdStarted,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdLockedIn,
						Choice: 0x02,
					},
					{
						State:  ThresholdActive,
						Choice: 0x02,
					},
					{
						State:  ThresholdActive,
						Choice: 0x02,
					},
				},
				// 1
				{
					{
						State:  ThresholdDefined,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdStarted,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdStarted,
						Choice: invalidChoice,
					},
					{
						State:  ThresholdFailed,
						Choice: 0x01,
					},
					{
						State:  ThresholdFailed,
						Choice: 0x01,
					},
					{
						State:  ThresholdFailed,
						Choice: 0x01,
					},
				},
			},
		},
	}

	for _, test := range tests {
		// Reset params.
		params = defaultParallelParams()
		// We have to reset the cache for every test.
		bc := &BlockChain{
			chainParams:      &params,
			deploymentCaches: newThresholdCaches(&params),
			index:            make(map[chainhash.Hash]*blockNode),
		}
		genesisNode := genesisBlockNode(&params)
		genesisNode.header.StakeVersion = test.startStakeVersion

		t.Logf("running: %v", test.name)

		var currentNode *blockNode
		currentNode = genesisNode
		currentTimestamp := time.Now()
		currentHeight := uint32(1)
		for k := range test.expectedState[0] {
			for i := uint32(0); i < test.voteBitsCounts[k].count; i++ {
				// Make up a header.
				header := &wire.BlockHeader{
					Version:      test.blockVersion,
					Height:       currentHeight,
					Nonce:        uint32(0),
					StakeVersion: test.startStakeVersion,
					Timestamp:    currentTimestamp,
				}
				hash := header.BlockHash()
				node := newBlockNode(header, &hash, 0,
					[]chainhash.Hash{}, []chainhash.Hash{},
					[]VoteVersionTuple{})
				node.height = int64(currentHeight)
				node.parent = currentNode

				// set stake versions and vote bits
				for x := 0; x < int(params.TicketsPerBlock); x++ {
					node.votes = append(node.votes,
						test.voteBitsCounts[k].vote)
				}

				currentNode = node
				bc.bestNode = currentNode

				// fake index
				bc.index[hash] = node

				currentHeight++
				currentTimestamp = currentTimestamp.Add(time.Second)
			}
			t.Logf("Height %v", currentNode.height)
			for i := range test.vote {
				ts, err := bc.ThresholdState(&currentNode.hash,
					posVersion, test.vote[i].Id)
				if err != nil {
					t.Fatalf("ThresholdState(%v): %v", k, err)
				}

				if ts != test.expectedState[i][k] {
					t.Fatalf("%v.%v (%v) got state %v "+
						"wanted state %v got choice "+
						"%v wanted choice %v",
						test.name, test.vote[i].Id, k,
						ts, test.expectedState[i][k],
						ts.Choice,
						test.expectedState[i][k].Choice)
				}
			}
		}
	}
}

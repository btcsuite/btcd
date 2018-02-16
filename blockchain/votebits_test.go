// Copyright (c) 2017-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
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
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	multipleChoice = chaincfg.Vote{
		Id:          "multiplechoice",
		Description: "Pick one",
		Mask:        0x70, // 0b0111 0000
		Choices: []chaincfg.Choice{
			{
				Id:          "Abstain",
				Description: "Abstain multiple choice",
				Bits:        0x0, // 0b0000 0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "one",
				Description: "Choice 1",
				Bits:        0x10, // 0b0001 0000
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "Vote against",
				Description: "Vote against all multiple ",
				Bits:        0x20, // 0b0010 0000
				IsAbstain:   false,
				IsNo:        true,
			},
			{
				Id:          "two",
				Description: "Choice 2",
				Bits:        0x30, // 0b0011 0000
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "three",
				Description: "Choice 3",
				Bits:        0x40, // 0b0100 0000
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "four",
				Description: "Choice 4",
				Bits:        0x50, // 0b0101 0000
				IsAbstain:   false,
				IsNo:        false,
			},
		},
	}
)

func defaultParams(vote chaincfg.Vote) chaincfg.Params {
	params := chaincfg.SimNetParams
	params.Deployments = make(map[uint32][]chaincfg.ConsensusDeployment)
	params.Deployments[posVersion] = []chaincfg.ConsensusDeployment{{
		Vote: vote,
		StartTime: uint64(time.Now().Add(time.Duration(
			params.RuleChangeActivationInterval) *
			time.Second).Unix()),
		ExpireTime: uint64(time.Now().Add(24 * time.Hour).Unix()),
	}}

	return params
}

func TestSerializeDeserialize(t *testing.T) {
	params := defaultParams(pedro)
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
		if deserialized.Vote.Choices[i].IsAbstain !=
			ourDeployment.Vote.Choices[i].IsAbstain {
			t.Fatalf("invalid IsAbstain %v got %v expected %v", i,
				deserialized.Vote.Choices[i].IsAbstain,
				ourDeployment.Vote.Choices[i].IsAbstain)
		}
		if deserialized.Vote.Choices[i].IsNo !=
			ourDeployment.Vote.Choices[i].IsNo {
			t.Fatalf("invalid IsNo %v got %v expected %v", i,
				deserialized.Vote.Choices[i].IsNo,
				ourDeployment.Vote.Choices[i].IsNo)
		}
	}
}

// TestNoQuorum ensures that the quorum behavior works as expected with no
// votes.
func TestNoQuorum(t *testing.T) {
	params := defaultParams(pedro)
	bc := newFakeChain(&params)
	node := bc.bestNode
	node.stakeVersion = posVersion

	// get to svi
	curTimestamp := time.Now()
	for i := uint32(0); i < uint32(params.StakeValidationHeight); i++ {
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}
	ts, err := bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(SVI): %v", err)
	}
	tse := ThresholdStateTuple{
		State:  ThresholdDefined,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to started
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval-1); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		appendFakeVotes(node, params.TicketsPerBlock, posVersion, 0x01)
		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(started): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to quorum - 1
	voteCount := uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			bits := uint16(0x01) // abstain
			if voteCount < params.RuleChangeActivationQuorum-1 {
				bits = 0x05 // vote no
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to exact quorum but with 75%%-1 yes votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			// 119 no, 41 yes -> 120 == 75% and 120 reaches quorum
			bits := uint16(0x01) // abstain
			quorum := params.RuleChangeActivationQuorum*
				params.RuleChangeActivationMultiplier/
				params.RuleChangeActivationDivisor - 1
			if voteCount < quorum {
				bits = 0x05 // vote no
			} else if voteCount < params.RuleChangeActivationQuorum {
				bits = 0x03 // vote yes
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to exact quorum with exactly 75% of votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			// 120 no, 40 yes -> 120 == 75% and 120 reaches quorum
			bits := uint16(0x01) // abstain
			quorum := params.RuleChangeActivationQuorum *
				params.RuleChangeActivationMultiplier /
				params.RuleChangeActivationDivisor
			if voteCount < quorum {
				bits = 0x05 // vote no
			} else if voteCount < params.RuleChangeActivationQuorum {
				bits = 0x03 // vote yes
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdFailed,
		Choice: 0x02,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}
}

// TestNoQuorum ensures that the quorum behavior works as expected with yes
// votes.
func TestYesQuorum(t *testing.T) {
	params := defaultParams(pedro)
	bc := newFakeChain(&params)
	node := bc.bestNode
	node.stakeVersion = posVersion

	// get to svi
	curTimestamp := time.Now()
	for i := uint32(0); i < uint32(params.StakeValidationHeight); i++ {
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}
	ts, err := bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(SVI): %v", err)
	}
	tse := ThresholdStateTuple{
		State:  ThresholdDefined,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to started
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval-1); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		appendFakeVotes(node, params.TicketsPerBlock, posVersion, 0x01)
		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(started): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to quorum - 1
	voteCount := uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			bits := uint16(0x01) // abstain
			if voteCount < params.RuleChangeActivationQuorum-1 {
				bits = 0x03 // vote yes
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to exact quorum but with 75%-1 yes votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			// 119 yes, 41 no -> 120 == 75% and 120 reaches quorum
			bits := uint16(0x01) // abstain
			quorum := params.RuleChangeActivationQuorum*
				params.RuleChangeActivationMultiplier/
				params.RuleChangeActivationDivisor - 1
			if voteCount < quorum {
				bits = 0x03 // vote yes
			} else if voteCount < params.RuleChangeActivationQuorum {
				bits = 0x05 // vote no
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%-1): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdStarted,
		Choice: invalidChoice,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}

	// get to exact quorum with exactly 75% of votes
	voteCount = uint32(0)
	for i := uint32(0); i < uint32(params.RuleChangeActivationInterval); i++ {
		// Set stake versions and vote bits.
		node = newFakeNode(node, powVersion, posVersion, 0, curTimestamp)
		for x := 0; x < int(params.TicketsPerBlock); x++ {
			// 120 yes, 40 no -> 120 == 75% and 120 reaches quorum
			bits := uint16(0x01) // abstain
			quorum := params.RuleChangeActivationQuorum *
				params.RuleChangeActivationMultiplier /
				params.RuleChangeActivationDivisor
			if voteCount < quorum {
				bits = 0x03 // vote yes
			} else if voteCount < params.RuleChangeActivationQuorum {
				bits = 0x05 // vote no
			}
			appendFakeVotes(node, 1, posVersion, bits)
			voteCount++
		}

		bc.bestNode = node
		bc.index.AddNode(node)
		curTimestamp = curTimestamp.Add(time.Second)
	}

	ts, err = bc.ThresholdState(&node.hash, posVersion, pedro.Id)
	if err != nil {
		t.Fatalf("ThresholdState(quorum 75%%): %v", err)
	}
	tse = ThresholdStateTuple{
		State:  ThresholdLockedIn,
		Choice: 0x01,
	}
	if ts != tse {
		t.Fatalf("expected %+v got %+v", ts, tse)
	}
}

// TestVoting ensure the overall voting of an agenda works as expected including
// a wide variety of conditions.
func TestVoting(t *testing.T) {
	params := defaultParams(chaincfg.Vote{})
	rci := params.RuleChangeActivationInterval
	svh := uint32(params.StakeValidationHeight)
	svi := uint32(params.StakeVersionInterval)

	type voteCount struct {
		vote  stake.VoteVersionTuple
		count uint32
	}

	tests := []struct {
		name              string
		vote              chaincfg.Vote
		blockVersion      int32
		startStakeVersion uint32
		end               func(time.Time) uint64
		voteBitsCounts    []voteCount
		expectedState     []ThresholdStateTuple
	}{
		{
			name:              "pedro too shallow",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh - 1,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "pedro greater PoS version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion - 1,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svi - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - svi,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 1,
			}},
		},
		{
			name:              "pedro greater PoS version calcStakeVersion",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion - 1,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    0x01,
				},
				count: svh + 2*svi - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci % svi,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 1,
			}},
		},
		{
			name:              "pedro smaller PoS version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion + 1,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}},
		},
		{
			name:              "pedro smaller PoW version",
			vote:              pedro,
			blockVersion:      powVersion - 1,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "pedro greater PoW version",
			vote:              pedro,
			blockVersion:      powVersion + 1,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}},
		},
		{
			name:              "pedro 100% yes, wrong version",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion + 1,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion + 1,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion + 1,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion + 1,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion + 1,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}},
		},
		{
			name:              "pedro 100% yes",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}},
		},
		{
			name:              "pedro 100% no",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x05,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x05,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdFailed,
				Choice: 0x02,
			}, {
				State:  ThresholdFailed,
				Choice: 0x02,
			}, {
				State:  ThresholdFailed,
				Choice: 0x02,
			}},
		},
		{
			name:              "pedro 100% invalid",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x06,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x06,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 0x01,
			}, {
				State:  ThresholdActive,
				Choice: 0x01,
			}},
		},
		{
			name:              "pedro 100% abstain",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "pedro expire before started",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion - 1,
			end: func(t time.Time) uint64 {
				return uint64(t.Add(time.Second *
					time.Duration(int64(svh)+int64(rci+rci/2))).Unix())
			},
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    0x05,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    0x05,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdFailed,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "pedro expire after started",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			end: func(t time.Time) uint64 {
				return uint64(t.Add(time.Second *
					time.Duration(int64(svh)+int64(rci+rci/2))).Unix())
			},
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x05,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x05,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdFailed,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "pedro overlap",
			vote:              pedro,
			blockVersion:      powVersion,
			startStakeVersion: posVersion - 1,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    0x01,
				},
				count: svh + 19*svi - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svi - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: uint32(rci) - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x03,
				},
				count: 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: uint32(rci),
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 1,
			}, {
				State:  ThresholdActive,
				Choice: 1,
			}},
		},
		{
			name:              "multiple choice 100% abstain",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}},
		},
		{
			name:              "multiple choice 100% no",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x21,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdFailed,
				Choice: 2,
			}},
		},
		{
			name:              "multiple choice 100% choice 1",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x11,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x11,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 1,
			}, {
				State:  ThresholdActive,
				Choice: 1,
			}, {
				State:  ThresholdActive,
				Choice: 1,
			}},
		},
		{
			name:              "multiple choice 100% choice 2",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x31,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x31,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 3,
			}, {
				State:  ThresholdActive,
				Choice: 3,
			}, {
				State:  ThresholdActive,
				Choice: 3,
			}},
		},
		{
			name:              "multiple choice 100% choice 3",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x41,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x41,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 4,
			}, {
				State:  ThresholdActive,
				Choice: 4,
			}, {
				State:  ThresholdActive,
				Choice: 4,
			}},
		},
		{
			name:              "multiple choice 100% choice 4",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x51,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x51,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdLockedIn,
				Choice: 5,
			}, {
				State:  ThresholdActive,
				Choice: 5,
			}, {
				State:  ThresholdActive,
				Choice: 5,
			}},
		},
		{
			name:              "multiple choice 100% choice 5 (invalid)",
			vote:              multipleChoice,
			blockVersion:      powVersion,
			startStakeVersion: posVersion,
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: svh,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x61,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x61,
				},
				count: rci,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: rci,
			}},
			expectedState: []ThresholdStateTuple{{
				State:  ThresholdDefined,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}, {
				State:  ThresholdStarted,
				Choice: invalidChoice,
			}},
		},
	}

	for _, test := range tests {
		// Reset params.
		params = defaultParams(test.vote)
		// We have to reset the cache for every test.
		bc := newFakeChain(&params)
		node := bc.bestNode
		node.stakeVersion = test.startStakeVersion

		t.Logf("running: %v", test.name)

		// Override start time.
		curTimestamp := time.Now()

		// Override expiration time.
		if test.end != nil {
			params.Deployments[posVersion][0].ExpireTime =
				test.end(curTimestamp)
		}

		t.Logf("curTimestamp %v start %v expiration %v",
			uint64(curTimestamp.Unix()),
			params.Deployments[posVersion][0].StartTime,
			params.Deployments[posVersion][0].ExpireTime)

		for k := range test.expectedState {
			for i := uint32(0); i < test.voteBitsCounts[k].count; i++ {
				// Set stake versions and vote bits.
				node = newFakeNode(node, test.blockVersion,
					test.startStakeVersion, 0, curTimestamp)
				vote := &test.voteBitsCounts[k].vote
				appendFakeVotes(node, params.TicketsPerBlock,
					vote.Version, vote.Bits)

				bc.bestNode = node
				bc.index.AddNode(node)
				curTimestamp = curTimestamp.Add(time.Second)
			}
			t.Logf("Height %v, Start time %v, curTime %v, delta %v",
				node.height, params.Deployments[4][0].StartTime,
				node.timestamp, node.timestamp-
					int64(params.Deployments[4][0].StartTime))
			ts, err := bc.ThresholdState(&node.hash, posVersion,
				test.vote.Id)
			if err != nil {
				t.Fatalf("ThresholdState(%v): %v", k, err)
			}
			if ts != test.expectedState[k] {
				t.Fatalf("%v.%v (%v) got state %+v wanted "+
					"state %+v", test.name, test.vote.Id, k,
					ts, test.expectedState[k])
			}
		}
	}
}

// Parallel test.
const (
	testDummy1ID    = "testdummy1"
	vbTestDummy1Yes = 0x04

	testDummy2ID   = "testdummy2"
	vbTestDummy2No = 0x08
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
			IsAbstain:   true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0002, // Bit 1
			IsAbstain:   false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0004, // Bit 2
			IsAbstain:   false,
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
			IsAbstain:   true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "vote no",
			Bits:        0x0008, // Bit 3
			IsAbstain:   false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "vote yes",
			Bits:        0x0010, // Bit 4
			IsAbstain:   false,
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

// TestParallelVoting ensures that two agendas running at the same time progress
// through the expected states.
func TestParallelVoting(t *testing.T) {
	params := defaultParallelParams()

	type voteCount struct {
		vote  stake.VoteVersionTuple
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
			voteBitsCounts: []voteCount{{
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: uint32(params.StakeValidationHeight),
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: params.RuleChangeActivationInterval - 1,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion - 1,
					Bits:    vbTestDummy1Yes | vbTestDummy2No,
				},
				count: params.RuleChangeActivationInterval,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    vbTestDummy1Yes | vbTestDummy2No,
				},
				count: params.RuleChangeActivationInterval,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: params.RuleChangeActivationInterval,
			}, {
				vote: stake.VoteVersionTuple{
					Version: posVersion,
					Bits:    0x01,
				},
				count: params.RuleChangeActivationInterval,
			}},
			expectedState: [][]ThresholdStateTuple{
				// 0
				{{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				}, {
					State:  ThresholdStarted,
					Choice: invalidChoice,
				}, {
					State:  ThresholdStarted,
					Choice: invalidChoice,
				}, {
					State:  ThresholdLockedIn,
					Choice: 0x02,
				}, {
					State:  ThresholdActive,
					Choice: 0x02,
				}, {
					State:  ThresholdActive,
					Choice: 0x02,
				}},
				// 1
				{{
					State:  ThresholdDefined,
					Choice: invalidChoice,
				}, {
					State:  ThresholdStarted,
					Choice: invalidChoice,
				}, {
					State:  ThresholdStarted,
					Choice: invalidChoice,
				}, {
					State:  ThresholdFailed,
					Choice: 0x01,
				}, {
					State:  ThresholdFailed,
					Choice: 0x01,
				}, {
					State:  ThresholdFailed,
					Choice: 0x01,
				}},
			},
		},
	}

	for _, test := range tests {
		// Reset params.
		params = defaultParallelParams()
		// We have to reset the cache for every test.
		bc := newFakeChain(&params)
		node := bc.bestNode
		node.stakeVersion = test.startStakeVersion

		curTimestamp := time.Now()
		for k := range test.expectedState[0] {
			for i := uint32(0); i < test.voteBitsCounts[k].count; i++ {
				// Set stake versions and vote bits.
				node = newFakeNode(node, test.blockVersion,
					test.startStakeVersion, 0, curTimestamp)
				vote := &test.voteBitsCounts[k].vote
				appendFakeVotes(node, params.TicketsPerBlock,
					vote.Version, vote.Bits)

				bc.bestNode = node
				bc.index.AddNode(node)
				curTimestamp = curTimestamp.Add(time.Second)
			}
			for i := range test.vote {
				ts, err := bc.ThresholdState(&node.hash,
					posVersion, test.vote[i].Id)
				if err != nil {
					t.Fatalf("ThresholdState(%v): %v", k, err)
				}

				if ts != test.expectedState[i][k] {
					t.Fatalf("%v.%v (%v) got state %+v "+
						"wanted state %+v",
						test.name, test.vote[i].Id, k,
						ts, test.expectedState[i][k])
				}
			}
		}
	}
}

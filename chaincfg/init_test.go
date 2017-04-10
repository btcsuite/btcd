// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"
)

var (
	consecMask = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0xa, // 0b1010 XXX not consecutive mask
		Choices: []Choice{
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

	tooManyChoices = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
			{
				Id:          "Maybe",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110
				IsAbstain:   false,
				IsNo:        false,
			},
			// XXX here we go out of mask bounds
			{
				Id:          "Hmmmm",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110 XXX invalid bits too
				IsAbstain:   false,
				IsNo:        false,
			},
		},
	}

	notConsec = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
			// XXX not consecutive
			{
				Id:          "Maybe",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110
				IsAbstain:   false,
				IsNo:        false,
			},
		},
	}

	invalidAbstain = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0,   // 0b0000
				IsAbstain:   false, // XXX this is the invalid bit
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

	invalidVoteBits = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
				Bits:        0xc, // 0b1100 XXX invalid bits
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	twoIsAbstain = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
				Bits:        0x2,  // 0b0010
				IsAbstain:   true, // XXX this is the invalid choice
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

	twoIsNo = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
				IsNo:        true, // XXX this is the invalid choice
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

	bothFlags = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
				Bits:        0x2,  // 0b0010
				IsAbstain:   true, // XXX this is the invalid choice
				IsNo:        true, // XXX this is the invalid choice
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

	dupChoice = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
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
				Id:          "Yes", // XXX this is the invalid ID
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}
)

func TestChoices(t *testing.T) {
	tests := []struct {
		name     string
		vote     Vote
		expected error
	}{
		{
			name:     "consecutive mask",
			vote:     consecMask,
			expected: ErrInvalidMask,
		},
		{
			name:     "not consecutive choices",
			vote:     notConsec,
			expected: ErrNotConsecutive,
		},
		{
			name:     "too many choices",
			vote:     tooManyChoices,
			expected: ErrTooManyChoices,
		},
		{
			name:     "invalid ignore",
			vote:     invalidAbstain,
			expected: ErrInvalidAbstain,
		},
		{
			name:     "invalid vote bits",
			vote:     invalidVoteBits,
			expected: ErrInvalidBits,
		},
		{
			name:     "2 IsAbstain",
			vote:     twoIsAbstain,
			expected: ErrInvalidIsAbstain,
		},
		{
			name:     "2 IsNo",
			vote:     twoIsNo,
			expected: ErrInvalidIsNo,
		},
		{
			name:     "both IsAbstain IsNo",
			vote:     bothFlags,
			expected: ErrInvalidBothFlags,
		},
		{
			name:     "duplicate choice id",
			vote:     dupChoice,
			expected: ErrDuplicateChoiceId,
		},
	}

	for _, test := range tests {
		t.Logf("running: %v", test.name)
		err := validateAgenda(test.vote)
		if err != test.expected {
			t.Fatalf("%v: got '%v' expected '%v'", test.name, err,
				test.expected)
		}
	}
}

var (
	dupVote = []ConsensusDeployment{
		{Vote: Vote{Id: "moo"}},
		{Vote: Vote{Id: "moo"}},
	}
)

func TestDeployments(t *testing.T) {
	tests := []struct {
		name        string
		deployments []ConsensusDeployment
		expected    error
	}{
		{
			name:        "duplicate vote id",
			deployments: dupVote,
			expected:    ErrDuplicateVoteId,
		},
	}

	for _, test := range tests {
		t.Logf("running: %v", test.name)
		_, err := validateDeployments(test.deployments)
		if err != test.expected {
			t.Fatalf("%v: got '%v' expected '%v'", test.name, err,
				test.expected)
		}
	}
}

// Copyright (c) 2014-2014 PPCD developers.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"regexp"
	"github.com/mably/btcchain"
	"github.com/mably/btcdb"
	"github.com/mably/btcjson"
	"github.com/mably/btcwire"
)

// getDifficultyRatio returns the latest PoW or PoS difficulty up to block sha.
func ppcGetDifficultyRatio(db btcdb.Db, sha *btcwire.ShaHash, proofOfStake bool) (float64, error) {
	bh, _, err := btcchain.GetLastBlockHeader(db, sha, proofOfStake)
	if err != nil {
		return 0, err
	}
	return getDifficultyRatio(bh.Bits), nil
}

// ppcHandleGetDifficulty implements the getdifficulty command.
func ppcHandleGetDifficulty(s *rpcServer, cmd btcjson.Cmd, closeChan <-chan struct{}) (interface{}, error) {
	sha, _, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting sha: %v", err)
		return nil, btcjson.ErrDifficulty
	}
	powDifficulty, err := ppcGetDifficultyRatio(s.server.db, sha, false) // ppc: PoW
	if err != nil {
		rpcsLog.Errorf("Error getting difficulty: %v", err)
		return nil, btcjson.ErrDifficulty
	}
	posDifficulty, err := ppcGetDifficultyRatio(s.server.db, sha, true) // ppc: PoS
	if err != nil {
		rpcsLog.Errorf("Error getting difficulty: %v", err)
		return nil, btcjson.ErrDifficulty
	}

	ret := &btcjson.GetDifficultyResult{
		ProofOfWork:    powDifficulty,
		ProofOfStake:   posDifficulty,
		SearchInterval: int32(0),
	}

	return ret, nil
}

func isIPv4(addr string) (bool, error) {
	return regexp.MatchString("^(([01]?[0-9]?[0-9]|2([0-4][0-9]|5[0-5]))\\.){3}([01]?[0-9]?[0-9]|2([0-4][0-9]|5[0-5]))$", addr)
}

func isLoopBackIPv4(addr string) (bool ,error) {
	return regexp.MatchString("^127\\.", addr)
}
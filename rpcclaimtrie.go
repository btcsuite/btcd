package main

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/lbryio/lbcd/btcjson"
	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/node"
	"github.com/lbryio/lbcd/claimtrie/normalization"
	"github.com/lbryio/lbcd/database"
	"github.com/lbryio/lbcd/txscript"
	"github.com/lbryio/lbcd/wire"
)

var claimtrieHandlers = map[string]commandHandler{
	"getchangesinblock":     handleGetChangesInBlock,
	"getclaimsforname":      handleGetClaimsForName,
	"getclaimsfornamebyid":  handleGetClaimsForNameByID,
	"getclaimsfornamebybid": handleGetClaimsForNameByBid,
	"getclaimsfornamebyseq": handleGetClaimsForNameBySeq,
	"normalize":             handleGetNormalized,
}

func handleGetChangesInBlock(s *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {

	c := cmd.(*btcjson.GetChangesInBlockCmd)
	hash, height, err := parseHashOrHeight(s, c.HashOrHeight)
	if err != nil {
		return nil, err
	}

	names, err := s.cfg.Chain.GetNamesChangedInBlock(height)
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCMisc,
			Message: "Message: " + err.Error(),
		}
	}

	return btcjson.GetChangesInBlockResult{
		Hash:   hash,
		Height: height,
		Names:  names,
	}, nil
}

func parseHashOrHeight(s *rpcServer, hashOrHeight *string) (string, int32, error) {
	if hashOrHeight == nil || len(*hashOrHeight) == 0 {

		if !s.cfg.Chain.IsCurrent() {
			return "", 0, &btcjson.RPCError{
				Code:    btcjson.ErrRPCClientInInitialDownload,
				Message: "Unable to query the chain tip during initial download",
			}
		}

		// just give them the latest block if a specific one wasn't requested
		best := s.cfg.Chain.BestSnapshot()
		return best.Hash.String(), best.Height, nil
	}

	ht, err := strconv.ParseInt(*hashOrHeight, 10, 32)
	if err == nil && len(*hashOrHeight) < 32 {
		hs, err := s.cfg.Chain.BlockHashByHeight(int32(ht))
		if err != nil {
			return "", 0, &btcjson.RPCError{
				Code:    btcjson.ErrRPCBlockNotFound,
				Message: "Unable to locate a block at height " + *hashOrHeight + ": " + err.Error(),
			}
		}
		return hs.String(), int32(ht), nil
	}

	hs, err := chainhash.NewHashFromStr(*hashOrHeight)
	if err != nil {
		return "", 0, &btcjson.RPCError{
			Code:    btcjson.ErrRPCInvalidParameter,
			Message: "Unable to parse a height or hash from " + *hashOrHeight + ": " + err.Error(),
		}
	}
	h, err := s.cfg.Chain.BlockHeightByHash(hs)
	if err != nil {
		return hs.String(), h, &btcjson.RPCError{
			Code:    btcjson.ErrRPCBlockNotFound,
			Message: "Unable to find a block with hash " + hs.String() + ": " + err.Error(),
		}
	}
	return hs.String(), h, nil
}

func handleGetClaimsForName(s *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {

	c := cmd.(*btcjson.GetClaimsForNameCmd)
	hash, height, err := parseHashOrHeight(s, c.HashOrHeight)
	if err != nil {
		return nil, err
	}

	name, n, err := s.cfg.Chain.GetClaimsForName(height, c.Name)
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCMisc,
			Message: "Message: " + err.Error(),
		}
	}

	var results []btcjson.ClaimResult
	for i := range n.Claims {
		cr, err := toClaimResult(s, int32(i), n, c.IncludeValues)
		if err != nil {
			return nil, err
		}
		results = append(results, cr)
	}

	return btcjson.GetClaimsForNameResult{
		Hash:               hash,
		Height:             height,
		LastTakeoverHeight: n.TakenOverAt,
		NormalizedName:     name,
		Claims:             results,
	}, nil
}

func handleGetClaimsForNameByID(s *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {

	c := cmd.(*btcjson.GetClaimsForNameByIDCmd)
	hash, height, err := parseHashOrHeight(s, c.HashOrHeight)
	if err != nil {
		return nil, err
	}

	name, n, err := s.cfg.Chain.GetClaimsForName(height, c.Name)
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCMisc,
			Message: "Message: " + err.Error(),
		}
	}

	var results []btcjson.ClaimResult
	for i := 0; i < len(n.Claims); i++ {
		for _, id := range c.PartialClaimIDs {
			if strings.HasPrefix(n.Claims[i].ClaimID.String(), id) {
				cr, err := toClaimResult(s, int32(i), n, c.IncludeValues)
				if err != nil {
					return nil, err
				}
				results = append(results, cr)
				break
			}
		}
	}

	return btcjson.GetClaimsForNameResult{
		Hash:               hash,
		Height:             height,
		LastTakeoverHeight: n.TakenOverAt,
		NormalizedName:     name,
		Claims:             results,
	}, nil
}

func handleGetClaimsForNameByBid(s *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {

	c := cmd.(*btcjson.GetClaimsForNameByBidCmd)
	hash, height, err := parseHashOrHeight(s, c.HashOrHeight)
	if err != nil {
		return nil, err
	}

	name, n, err := s.cfg.Chain.GetClaimsForName(height, c.Name)
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCMisc,
			Message: "Message: " + err.Error(),
		}
	}

	var results []btcjson.ClaimResult
	for _, b := range c.Bids { // claims are already sorted in bid order
		if b >= 0 && int(b) < len(n.Claims) {
			cr, err := toClaimResult(s, b, n, c.IncludeValues)
			if err != nil {
				return nil, err
			}
			results = append(results, cr)
		}
	}

	return btcjson.GetClaimsForNameResult{
		Hash:               hash,
		Height:             height,
		LastTakeoverHeight: n.TakenOverAt,
		NormalizedName:     name,
		Claims:             results,
	}, nil
}

func handleGetClaimsForNameBySeq(s *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {

	c := cmd.(*btcjson.GetClaimsForNameBySeqCmd)
	hash, height, err := parseHashOrHeight(s, c.HashOrHeight)
	if err != nil {
		return nil, err
	}

	name, n, err := s.cfg.Chain.GetClaimsForName(height, c.Name)
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCMisc,
			Message: "Message: " + err.Error(),
		}
	}

	sm := map[int32]bool{}
	for _, seq := range c.Sequences {
		sm[seq] = true
	}

	var results []btcjson.ClaimResult
	for i := 0; i < len(n.Claims); i++ {
		if sm[n.Claims[i].Sequence] {
			cr, err := toClaimResult(s, int32(i), n, c.IncludeValues)
			if err != nil {
				return nil, err
			}
			results = append(results, cr)
		}
	}

	return btcjson.GetClaimsForNameResult{
		Hash:               hash,
		Height:             height,
		LastTakeoverHeight: n.TakenOverAt,
		NormalizedName:     name,
		Claims:             results,
	}, nil
}

func toClaimResult(s *rpcServer, i int32, n *node.Node, includeValues *bool) (btcjson.ClaimResult, error) {
	claim := n.Claims[i]
	address, value, err := lookupValue(s, claim.OutPoint, includeValues)
	supports, err := toSupportResults(s, i, n, includeValues)
	effectiveAmount := n.SupportSums[claim.ClaimID.Key()] // should only be active supports
	if claim.Status == node.Activated {
		effectiveAmount += claim.Amount
	}
	return btcjson.ClaimResult{
		ClaimID:         claim.ClaimID.String(),
		Height:          claim.AcceptedAt,
		ValidAtHeight:   claim.ActiveAt,
		TXID:            claim.OutPoint.Hash.String(),
		N:               claim.OutPoint.Index,
		Bid:             i, // assuming sorted by bid
		Amount:          claim.Amount,
		EffectiveAmount: effectiveAmount,
		Sequence:        claim.Sequence,
		Supports:        supports,
		Address:         address,
		Value:           value,
	}, err
}

func toSupportResults(s *rpcServer, i int32, n *node.Node, includeValues *bool) ([]btcjson.SupportResult, error) {
	var results []btcjson.SupportResult
	c := n.Claims[i]
	for _, sup := range n.Supports {
		if sup.Status == node.Activated && c.ClaimID == sup.ClaimID {
			address, value, err := lookupValue(s, sup.OutPoint, includeValues)
			if err != nil {
				return results, err
			}
			results = append(results, btcjson.SupportResult{
				TXID:          sup.OutPoint.Hash.String(),
				N:             sup.OutPoint.Index,
				Height:        sup.AcceptedAt,
				ValidAtHeight: sup.ActiveAt,
				Amount:        sup.Amount,
				Value:         value,
				Address:       address,
			})
		}
	}
	return results, nil
}

func lookupValue(s *rpcServer, outpoint wire.OutPoint, includeValues *bool) (string, string, error) {
	if includeValues == nil || !*includeValues {
		return "", "", nil
	}
	// TODO: maybe use addrIndex if the txIndex is not available

	if s.cfg.TxIndex == nil {
		return "", "", &btcjson.RPCError{
			Code: btcjson.ErrRPCNoTxInfo,
			Message: "The transaction index must be " +
				"enabled to query the blockchain " +
				"(specify --txindex)",
		}
	}

	txHash := &outpoint.Hash
	blockRegion, err := s.cfg.TxIndex.TxBlockRegion(txHash)
	if err != nil {
		context := "Failed to retrieve transaction location"
		return "", "", internalRPCError(err.Error(), context)
	}
	if blockRegion == nil {
		return "", "", rpcNoTxInfoError(txHash)
	}

	// Load the raw transaction bytes from the database.
	var txBytes []byte
	err = s.cfg.DB.View(func(dbTx database.Tx) error {
		var err error
		txBytes, err = dbTx.FetchBlockRegion(blockRegion)
		return err
	})
	if err != nil {
		return "", "", rpcNoTxInfoError(txHash)
	}

	// Deserialize the transaction
	var msgTx wire.MsgTx
	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		context := "Failed to deserialize transaction"
		return "", "", internalRPCError(err.Error(), context)
	}

	txo := msgTx.TxOut[outpoint.Index]
	cs, err := txscript.ExtractClaimScript(txo.PkScript)
	if err != nil {
		context := "Failed to decode the claim script"
		return "", "", internalRPCError(err.Error(), context)
	}

	_, addresses, _, _ := txscript.ExtractPkScriptAddrs(txo.PkScript[cs.Size:], s.cfg.ChainParams)
	return addresses[0].EncodeAddress(), hex.EncodeToString(cs.Value), nil
}

func handleGetNormalized(_ *rpcServer, cmd interface{}, _ <-chan struct{}) (interface{}, error) {
	c := cmd.(*btcjson.GetNormalizedCmd)
	r := btcjson.GetNormalizedResult{
		NormalizedName: string(normalization.Normalize([]byte(c.Name))),
	}
	return r, nil
}

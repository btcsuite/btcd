package main

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcjson"
)

func TestRpcChainRuleError(t *testing.T) {
	t.Parallel()

	if got := rpcChainRuleError(nil); got != nil {
		t.Fatalf("nil -> %v", got)
	}

	rule := blockchain.RuleError{
		ErrorCode:   blockchain.ErrWitnessExcised,
		Description: "block is witness-excised",
	}
	got := rpcChainRuleError(rule)
	rpcErr, ok := got.(*btcjson.RPCError)
	if !ok {
		t.Fatalf("got %T, want *btcjson.RPCError", got)
	}
	if rpcErr.Code != btcjson.ErrRPCVerify {
		t.Fatalf("code=%v, want ErrRPCVerify", rpcErr.Code)
	}
	if rpcErr.Message != rule.Description {
		t.Fatalf("message=%q, want %q", rpcErr.Message, rule.Description)
	}
}

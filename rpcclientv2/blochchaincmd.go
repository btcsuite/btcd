package rpcclientv2

import (
	"encoding/json"
)

type GetDifficultyRpcCmd struct{}

func (u GetDifficultyRpcCmd) Name() string {
	return "getdifficulty"
}

func (u GetDifficultyRpcCmd) Marshal() (json.RawMessage, error) {
	return json.Marshal(u)
}

type FutureGetDifficulty chan *Response

type GetDifficultyResult float64

func (r FutureGetDifficulty) Receive() (GetDifficultyResult, error) {
	res, err := ReceiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as a float64.
	var difficulty float64
	err = json.Unmarshal(res, &difficulty)
	if err != nil {
		return 0, err
	}
	return GetDifficultyResult(difficulty), nil
}

type GetDifficultyRpcCall struct {
}

func (u *GetDifficultyRpcCall) MapReq(arg string) GetDifficultyRpcCmd {
	return GetDifficultyRpcCmd{}
}

func (u *GetDifficultyRpcCall) MapResp(resp chan *Response) MessageFuture[GetDifficultyResult] {
	f := FutureGetDifficulty(resp)

	return &f
}

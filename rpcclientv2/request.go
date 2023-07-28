package rpcclientv2

import (
	"encoding/json"
)

type Response struct {
	Resp json.RawMessage
	Err  error
}

type Command interface {
	Name() string
	Marshal() (json.RawMessage, error)
}

type Client interface {
	SendCmd(cmd Command) chan *Response
}

type MessageFuture[M any] interface {
	Receive() (M, error)
}

func ReceiveFuture(f chan *Response) (json.RawMessage, error) {
	// Wait for a response on the returned channel.
	r := <-f
	return r.Resp, r.Err
}

type RpcCall[Args any, Cmd Command, Resp any] interface {
	MapReq(arg Args) Cmd

	MapResp(resp chan *Response) MessageFuture[Resp]
}

func AsyncRequest[Args any, Cmd Command, Resp any](
	client Client, cmdReq RpcCall[Args, Cmd, Resp], args Args) MessageFuture[Resp] {

	jsonCmd := cmdReq.MapReq(args)

	resp := client.SendCmd(jsonCmd)

	return cmdReq.MapResp(resp)
}

type FakeClient struct {
}

func (m FakeClient) SendCmd(cmd Command) chan *Response {
	responseChan := make(chan *Response, 1)

	result := Response{
		Resp: []byte("561651.5"),
		Err:  nil,
	}
	responseChan <- &result
	return responseChan
}

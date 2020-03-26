package rpcclient

import (
	"encoding/json"

	"github.com/btcsuite/btcd/btcjson"
)

// FutureGetZmqNotificationsResult is a future promise to deliver the result of
// a GetZmqNotifications RPC invocation
type FutureGetZmqNotificationsResult chan *Response

// Receive waits for the response promised by the future and returns the unmarshalled
// response, or an error if the request was unsuccessful.
func (r FutureGetZmqNotificationsResult) Receive() (btcjson.GetZmqNotificationResult, error) {
	res, err := ReceiveFuture(r)
	if err != nil {
		return nil, err
	}
	var notifications btcjson.GetZmqNotificationResult
	if err := json.Unmarshal(res, &notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

// GetZmqNotificationsAsync returns an instance ofa type that can be used to get
// the result of a custom RPC request at some future time by invoking the Receive
// function on the returned instance.
//
// See GetZmqNotifications for the blocking version and more details.
func (c *Client) GetZmqNotificationsAsync() FutureGetZmqNotificationsResult {
	return c.SendCmd(btcjson.NewGetZmqNotificationsCmd())
}

// GetZmqNotifications returns information about the active ZeroMQ notifications.
func (c *Client) GetZmqNotifications() (btcjson.GetZmqNotificationResult, error) {
	return c.GetZmqNotificationsAsync().Receive()
}

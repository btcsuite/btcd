package rpcclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

// TestUnmarshalGetBlockChainInfoResult ensures that the SoftForks and
// UnifiedSoftForks fields of GetBlockChainInfoResult are properly unmarshaled
// when using the expected backend version.
func TestUnmarshalGetBlockChainInfoResultSoftForks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    BackendVersion
		res        []byte
		compatible bool
	}{
		{
			name:       "bitcoind < 0.19.0 with separate softforks",
			version:    BitcoindPre19,
			res:        []byte(`{"softforks": [{"version": 2}]}`),
			compatible: true,
		},
		{
			name:       "bitcoind >= 0.19.0 with separate softforks",
			version:    BitcoindPre22,
			res:        []byte(`{"softforks": [{"version": 2}]}`),
			compatible: false,
		},
		{
			name:       "bitcoind < 0.19.0 with unified softforks",
			version:    BitcoindPre19,
			res:        []byte(`{"softforks": {"segwit": {"type": "bip9"}}}`),
			compatible: false,
		},
		{
			name:       "bitcoind >= 0.19.0 with unified softforks",
			version:    BitcoindPre22,
			res:        []byte(`{"softforks": {"segwit": {"type": "bip9"}}}`),
			compatible: true,
		},
	}

	for _, test := range tests {
		success := t.Run(test.name, func(t *testing.T) {
			// We'll start by unmarshaling the JSON into a struct.
			// The SoftForks and UnifiedSoftForks field should not
			// be set yet, as they are unmarshaled within a
			// different function.
			info, err := unmarshalPartialGetBlockChainInfoResult(test.res)
			if err != nil {
				t.Fatal(err)
			}
			if info.SoftForks != nil {
				t.Fatal("expected SoftForks to be empty")
			}
			if info.UnifiedSoftForks != nil {
				t.Fatal("expected UnifiedSoftForks to be empty")
			}

			// Proceed to unmarshal the softforks of the response
			// with the expected version. If the version is
			// incompatible with the response, then this should
			// fail.
			err = unmarshalGetBlockChainInfoResultSoftForks(
				info, test.version, test.res,
			)
			if test.compatible && err != nil {
				t.Fatalf("unable to unmarshal softforks: %v", err)
			}
			if !test.compatible && err == nil {
				t.Fatal("expected to not unmarshal softforks")
			}
			if !test.compatible {
				return
			}

			// If the version is compatible with the response, we
			// should expect to see the proper softforks field set.
			if test.version == BitcoindPre22 &&
				info.SoftForks != nil {
				t.Fatal("expected SoftForks to be empty")
			}
			if test.version == BitcoindPre19 &&
				info.UnifiedSoftForks != nil {
				t.Fatal("expected UnifiedSoftForks to be empty")
			}
		})
		if !success {
			return
		}
	}
}

func TestFutureGetBlockCountResultReceiveErrors(t *testing.T) {
	responseChan := FutureGetBlockCountResult(make(chan *Response))
	response := Response{
		result: []byte{},
		err:    errors.New("blah blah something bad happened"),
	}
	go func() {
		responseChan <- &response
	}()

	_, err := responseChan.Receive()
	if err == nil || err.Error() != "blah blah something bad happened" {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestFutureGetBlockCountResultReceiveMarshalsResponseCorrectly(t *testing.T) {
	responseChan := FutureGetBlockCountResult(make(chan *Response))
	response := Response{
		result: []byte{0x36, 0x36},
		err:    nil,
	}
	go func() {
		responseChan <- &response
	}()

	res, err := responseChan.Receive()
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if res != 66 {
		t.Fatalf("unexpected response: %d (0x%X)", res, res)
	}
}

func TestClientConnectedToWSServerRunner(t *testing.T) {
	type TestTableItem struct {
		Name     string
		TestCase func(t *testing.T)
	}

	testTable := []TestTableItem{
		TestTableItem{
			Name: "TestGetChainTxStatsAsyncSuccessTx",
			TestCase: func(t *testing.T) {
				client, serverReceivedChannel, cleanup := makeClient(t)
				defer cleanup()
				client.GetChainTxStatsAsync()

				message := <-serverReceivedChannel
				if message != "{\"jsonrpc\":\"1.0\",\"method\":\"getchaintxstats\",\"params\":[],\"id\":1}" {
					t.Fatalf("received unexpected message: %s", message)
				}
			},
		},
		TestTableItem{
			Name: "TestGetChainTxStatsAsyncShutdownError",
			TestCase: func(t *testing.T) {
				client, _, cleanup := makeClient(t)
				defer cleanup()

				// a bit of a hack here: since there are multiple places where we read
				// from the shutdown channel, and it is not buffered, ensure that a shutdown
				// message is sent every time it is read from, this will ensure that
				// when client.GetChainTxStatsAsync() gets called, it hits the non-blocking
				// read from the shutdown channel
				go func() {
					type shutdownMessage struct{}
					for {
						client.shutdown <- shutdownMessage{}
					}
				}()

				var response *Response = nil

				for response == nil {
					respChan := client.GetChainTxStatsAsync()
					select {
					case response = <-respChan:
					default:
					}
				}

				if response.err == nil || response.err.Error() != "the client has been shutdown" {
					t.Fatalf("unexpected error: %s", response.err.Error())
				}
			},
		},
		TestTableItem{
			Name: "TestGetBestBlockHashAsync",
			TestCase: func(t *testing.T) {
				client, serverReceivedChannel, cleanup := makeClient(t)
				defer cleanup()
				ch := client.GetBestBlockHashAsync()

				message := <-serverReceivedChannel
				if message != "{\"jsonrpc\":\"1.0\",\"method\":\"getbestblockhash\",\"params\":[],\"id\":1}" {
					t.Fatalf("received unexpected message: %s", message)
				}

				expectedResponse := Response{}

				wg := sync.WaitGroup{}

				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						client.requestLock.Lock()
						if client.requestList.Len() > 0 {
							r := client.requestList.Back()
							r.Value.(*jsonRequest).responseChan <- &expectedResponse
							client.requestLock.Unlock()
							return
						}
						client.requestLock.Unlock()
					}
				}()

				response := <-ch

				if &expectedResponse != response {
					t.Fatalf("received unexepcted response")
				}

				// ensure the goroutine created in this test exists,
				// the test is ran with a timeout
				wg.Wait()
			},
		},
	}

	// since these tests rely on concurrency, ensure there is a resonable timeout
	// that they should run within
	for _, testCase := range testTable {
		done := make(chan bool)

		go func() {
			t.Run(testCase.Name, testCase.TestCase)
			done <- true
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout exceeded for: %s", testCase.Name)
		}
	}
}

func makeClient(t *testing.T) (*Client, chan string, func()) {
	serverReceivedChannel := make(chan string)
	s := httptest.NewServer(http.HandlerFunc(makeUpgradeOnConnect(serverReceivedChannel)))
	url := strings.TrimPrefix(s.URL, "http://")

	config := ConnConfig{
		DisableTLS: true,
		User:       "username",
		Pass:       "password",
		Host:       url,
	}

	client, err := New(&config, nil)
	if err != nil {
		t.Fatalf("error when creating new client %s", err.Error())
	}
	return client, serverReceivedChannel, func() {
		s.Close()
	}
}

func makeUpgradeOnConnect(ch chan string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				break
			}

			go func() {
				ch <- string(message)
			}()
		}
	}
}

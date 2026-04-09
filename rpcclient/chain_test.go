package rpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
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
			// We'll start by unmarshalling the JSON into a struct.
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
		{
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
		{
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
		{
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
					t.Fatalf("received unexpected response")
				}

				// ensure the goroutine created in this test exists,
				// the test is ran with a timeout
				wg.Wait()
			},
		},
	}

	// since these tests rely on concurrency, ensure there is a reasonable timeout
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

// TestJSONStringParsing tests the old vs. the two new methods for parsing a
// JSON string.
func TestJSONStringParsing(t *testing.T) {
	testCases := []string{
		"\"\"",
		"\"foo\"",
		"\"5152dcbe2d94dd2c66008b1281157ff44156d146c83bf3a182ba5b32ce" +
			"e79f5d\"",
		"\"010000004860eb18bf1b1620e37e9490fc8a427514416fd75159ab8668" +
			"8e9a8300000000d5fdcc541e25de1c7a5addedf24858b8bb665c" +
			"9f36ef744ee42c316022c90f9bb0bc6649ffff001d08d2bd6101" +
			"0100000001000000000000000000000000000000000000000000" +
			"0000000000000000000000ffffffff0704ffff001d010bffffff" +
			"ff0100f2052a010000004341047211a824f55b505228e4c3d519" +
			"4c1fcfaa15a456abdf37f9b9d97a4040afc073dee6c89064984f" +
			"03385237d92167c13e236446b417ab79a0fcae412ae3316b77ac" +
			"00000000\"",
	}

	oldMethod := func(res []byte) (string, error) {
		// Unmarshal result as a string.
		var resultStr string
		err := json.Unmarshal(res, &resultStr)
		if err != nil {
			return "", err
		}

		return resultStr, nil
	}
	newMethod := func(res []byte) (string, error) {
		return parseJSONString(res), nil
	}
	newMethodReader := func(res []byte) (string, error) {
		allBytes, err := io.ReadAll(parseJSONStringReader(res))
		if err != nil {
			return "", err
		}

		return string(allBytes), nil
	}

	for _, testCase := range testCases {
		t.Run(testCase, func(t *testing.T) {
			oldResult, err := oldMethod([]byte(testCase))
			require.NoError(t, err)

			newResult, err := newMethod([]byte(testCase))
			require.NoError(t, err)

			require.Equal(t, oldResult, newResult)

			newResult2, err := newMethodReader([]byte(testCase))
			require.NoError(t, err)

			require.Equal(t, oldResult, newResult2)
		})
	}
}

// BenchmarkParseJSONString benchmarks the old vs. the two new methods for
// parsing a JSON string.
func BenchmarkParseJSONString(b *testing.B) {
	// This is block 2 from mainnet.
	payload := []byte(
		"\"010000004860eb18bf1b1620e37e9490fc8a427514416fd75159ab8668" +
			"8e9a8300000000d5fdcc541e25de1c7a5addedf24858b8bb665c" +
			"9f36ef744ee42c316022c90f9bb0bc6649ffff001d08d2bd6101" +
			"0100000001000000000000000000000000000000000000000000" +
			"0000000000000000000000ffffffff0704ffff001d010bffffff" +
			"ff0100f2052a010000004341047211a824f55b505228e4c3d519" +
			"4c1fcfaa15a456abdf37f9b9d97a4040afc073dee6c89064984f" +
			"03385237d92167c13e236446b417ab79a0fcae412ae3316b77ac" +
			"00000000\"",
	)

	b.ResetTimer()
	b.Run("unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var s string
			if err := json.Unmarshal(payload, &s); err != nil {
				b.Fatal(err)
			}

			serializedBlock, err := hex.DecodeString(s)
			if err != nil {
				b.Fatal(err)
			}

			var msgBlock wire.MsgBlock
			err = msgBlock.Deserialize(bytes.NewReader(
				serializedBlock,
			))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("parseJSONString", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s := parseJSONString(payload)

			var msgBlock wire.MsgBlock
			err := msgBlock.Deserialize(hex.NewDecoder(
				strings.NewReader(s)),
			)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("parseJSONStringReader", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := parseJSONStringReader(payload)

			var msgBlock wire.MsgBlock
			err := msgBlock.Deserialize(hex.NewDecoder(r))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
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

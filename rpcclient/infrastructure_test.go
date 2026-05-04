package rpcclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postRoundTripFunc adapts a function to implement http.RoundTripper.
type postRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip invokes the wrapped test transport function.
func (f postRoundTripFunc) RoundTrip(
	req *http.Request) (*http.Response, error) {

	return f(req)
}

// cancelOnReadBody is a test response body that blocks reads until context
// cancellation is observed.
type cancelOnReadBody struct {
	// ctx is the request context that drives cancellation.
	ctx context.Context
	// readStarted is closed when the first read call starts.
	readStarted chan struct{}
	// stage names the part of the request flow waiting on cancellation.
	stage string
}

// Read blocks until the request context is canceled, then returns that error.
func (b *cancelOnReadBody) Read(_ []byte) (int, error) {
	select {
	case <-b.readStarted:
	default:
		close(b.readStarted)
	}

	return 0, waitForRequestContextCancellation(b.ctx, b.stage)
}

// Close implements io.Closer for the test body.
func (b *cancelOnReadBody) Close() error {
	return nil
}

// newPostModeTestClient builds a minimal HTTP POST-mode client for transport
// behavior tests.
func newPostModeTestClient(rt http.RoundTripper) *Client {
	return &Client{
		config: &ConnConfig{
			Host:         "127.0.0.1:8332",
			User:         "user",
			Pass:         "pass",
			DisableTLS:   true,
			HTTPPostMode: true,
		},
		httpClient: &http.Client{
			Transport: rt,
		},
	}
}

// newPostTestRequest creates a minimal JSON-RPC request used by POST handler
// tests.
func newPostTestRequest() *jsonRequest {
	body := `{"jsonrpc":"1.0","id":1,"method":"getblockcount","params":[]}`

	return &jsonRequest{
		id:             1,
		method:         "getblockcount",
		marshalledJSON: []byte(body),
		responseChan:   make(chan *Response, 1),
	}
}

// waitForRequestContextCancellation bounds shutdown waits so a missing request
// context propagation becomes a symptomatic test failure instead of a hang.
func waitForRequestContextCancellation(
	ctx context.Context, stage string) error {

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("request context was not canceled during %s",
			stage)
	}
}

// sendPostShutdownScenario describes one shutdown path that should fail if the
// request stops honoring shutdown cancellation.
type sendPostShutdownScenario struct {
	// name is the subtest name for the shutdown path.
	name string

	// tries is the retry count passed to the POST send helper under test.
	tries int

	// newClient builds a POST-mode client whose transport triggers the
	// shutdown path for this scenario.
	newClient func(context.CancelCauseFunc, *int32) *Client

	// wantAttempts is the expected number of transport attempts before the
	// scenario terminates.
	wantAttempts int32

	// wantErrContains is an optional substring that must appear in the raw
	// helper error for this scenario.
	wantErrContains string
}

// sendPostShutdownScenarios enumerates the shutdown-triggered regression
// scenarios shared by the helper-level and wrapper-level POST tests.
var sendPostShutdownScenarios = []sendPostShutdownScenario{
	{
		name:  "during_retry_backoff",
		tries: 2,
		newClient: func(cancel context.CancelCauseFunc,
			attempts *int32) *Client {

			return newPostModeTestClient(postRoundTripFunc(
				func(*http.Request) (*http.Response, error) {
					if atomic.AddInt32(attempts, 1) == 1 {
						cancel(ErrClientShutdown)
					}

					return nil, errors.New(
						"transient transport error",
					)
				},
			))
		},
		wantAttempts: 1,
	},
	{
		name:  "on_final_retry",
		tries: 2,
		newClient: func(cancel context.CancelCauseFunc,
			attempts *int32) *Client {

			return newPostModeTestClient(postRoundTripFunc(
				func(req *http.Request) (*http.Response, error) {
					current := atomic.AddInt32(attempts, 1)
					if current == 1 {
						return nil, errors.New(
							"transient transport error",
						)
					}

					// This keeps the case tied to request-context
					// propagation instead of injecting context.Canceled
					// directly from the fake transport.
					cancel(ErrClientShutdown)
					return nil, waitForRequestContextCancellation(
						req.Context(), "final retry",
					)
				},
			))
		},
		wantAttempts: 2,
	},
	{
		name:  "during_body_read",
		tries: 1,
		newClient: func(cancel context.CancelCauseFunc,
			attempts *int32) *Client {

			readStarted := make(chan struct{})
			go func() {
				<-readStarted
				cancel(ErrClientShutdown)
			}()

			return newPostModeTestClient(postRoundTripFunc(
				func(req *http.Request) (*http.Response, error) {
					atomic.AddInt32(attempts, 1)

					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body: &cancelOnReadBody{
							ctx:         req.Context(),
							readStarted: readStarted,
							stage:       "body read",
						},
					}, nil
				},
			))
		},
		wantAttempts:    1,
		wantErrContains: "error reading json reply",
	},
}

// TestParseAddressString checks different variation of supported and
// unsupported addresses.
func TestParseAddressString(t *testing.T) {
	t.Parallel()

	// Using localhost only to avoid network calls.
	testCases := []struct {
		name          string
		addressString string
		expNetwork    string
		expAddress    string
		expErrStr     string
	}{
		{
			name:          "localhost",
			addressString: "localhost",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ip",
			addressString: "127.0.0.1",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ipv6",
			addressString: "::1",
			expNetwork:    "tcp",
			expAddress:    "[::1]:0",
		},
		{
			name:          "localhost and port",
			addressString: "localhost:80",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "localhost ipv6 and port",
			addressString: "[::1]:80",
			expNetwork:    "tcp",
			expAddress:    "[::1]:80",
		},
		{
			name:          "colon and port",
			addressString: ":80",
			expNetwork:    "tcp",
			expAddress:    ":80",
		},
		{
			name:          "colon only",
			addressString: ":",
			expNetwork:    "tcp",
			expAddress:    ":0",
		},
		{
			name:          "localhost and path",
			addressString: "localhost/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost port and path",
			addressString: "localhost:80/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "unix prefix",
			addressString: "unix://the/rest/of/the/path",
			expNetwork:    "unix",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "unix prefix",
			addressString: "unixpacket://the/rest/of/the/path",
			expNetwork:    "unixpacket",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "error http prefix",
			addressString: "http://localhost:1010",
			expErrStr:     "unsupported protocol in address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := ParseAddressString(tc.addressString)
			if tc.expErrStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrStr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expNetwork, addr.Network())
				require.Equal(t, tc.expAddress, addr.String())
			}
		})
	}
}

// TestSendPostRequestWithRetrySuccess ensures that
// sendPostRequestWithRetry returns a decoded result and no error on
// a successful response.
func TestSendPostRequestWithRetrySuccess(t *testing.T) {
	client := newPostModeTestClient(postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"result":1,"error":null,"id":1}`,
				)),
			}, nil
		},
	))
	jReq := newPostTestRequest()

	result, err := sendPostRequestWithRetry(
		context.Background(), jReq, 1, client.httpClient, client.config,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []byte("1"), result)
}

// TestSendPostRequestWithRetryShutdown keeps the shutdown regression cases in
// one table while preserving a distinct symptomatic failure for each path.
func TestSendPostRequestWithRetryShutdown(t *testing.T) {
	for _, tc := range sendPostShutdownScenarios {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int32
			ctx, cancel := context.WithCancelCause(context.Background())
			client := tc.newClient(cancel, &attempts)
			jReq := newPostTestRequest()

			result, err := sendPostRequestWithRetry(
				ctx, jReq, tc.tries, client.httpClient, client.config,
				false,
			)
			require.Nil(t, result)
			require.ErrorIs(t, err, context.Canceled)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
			require.EqualValues(t, tc.wantAttempts,
				atomic.LoadInt32(&attempts))
		})
	}
}

// TestHTTPPostShutdownInterruptsPendingRequest ensures that a client operating
// in HTTP POST mode can interrupt an in-flight request during shutdown.
func TestHTTPPostShutdownInterruptsPendingRequest(t *testing.T) {
	t.Parallel()

	// Start a local TCP listener that accepts exactly one HTTP request and
	// then blocks until the client side closes the connection.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// requestAccepted signals when the test server has accepted the
	// client's connection.
	requestAccepted := make(chan struct{})

	// serverDone signals when the server goroutine has exited.
	serverDone := make(chan struct{})

	// Run a minimum server goroutine. It accepts one connection and drains
	// the request stream without replying so the client request stays in
	// flight.
	go func() {
		defer close(serverDone)

		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() {
			err := conn.Close()
			assert.NoError(t, err)
		}()

		close(requestAccepted)

		_, _ = io.Copy(io.Discard, conn)
	}()

	// Ensure the listener is closed and the server goroutine exits.
	t.Cleanup(func() {
		err := listener.Close()
		require.NoError(t, err)
		<-serverDone
	})

	// Configure a POST-mode client against the local listener.
	connCfg := &ConnConfig{
		Host:         listener.Addr().String(),
		User:         "user",
		Pass:         "pass",
		DisableTLS:   true,
		HTTPPostMode: true,
	}

	// Start the client and register cleanup for idempotent shutdown.
	client, err := New(connCfg, nil)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	// Launch one async request that should remain pending until shutdown.
	future := client.GetBlockCountAsync()

	// Ensure the server sees the request before we initiate shutdown.
	select {
	case <-requestAccepted:

	case <-time.After(2 * time.Second):
		t.Fatalf("server did not accept client connection")
	}

	// The request should remain pending until shutdown is requested.
	select {
	case <-future:
		t.Fatalf("expected request to remain pending until shutdown")

	case <-time.After(100 * time.Millisecond):
	}

	client.Shutdown()

	waitDone := make(chan struct{})
	go func() {
		client.WaitForShutdown()
		close(waitDone)
	}()

	// Wait for shutdown to complete before asserting the final error.
	select {
	case <-waitDone:

	case <-time.After(5 * time.Second):
		t.Fatalf("client shutdown did not complete")
	}

	result, err := future.Receive()
	require.Zero(t, result)
	require.ErrorContains(t, err, ErrClientShutdown.Error())
}

// TestSendPostRequestAndRespondShutdown reuses the helper-level shutdown cases
// to verify the client-facing contract: each one must surface
// ErrClientShutdown on the response channel.
func TestSendPostRequestAndRespondShutdown(t *testing.T) {
	for _, tc := range sendPostShutdownScenarios {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int32
			ctx, cancel := context.WithCancelCause(context.Background())
			client := tc.newClient(cancel, &attempts)
			jReq := newPostTestRequest()

			go client.sendPostRequestAndRespond(ctx, jReq, tc.tries)

			select {
			case resp := <-jReq.responseChan:
				require.ErrorIs(t, resp.err, ErrClientShutdown)
				require.Nil(t, resp.result)
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for response")
			}

			require.EqualValues(t, tc.wantAttempts,
				atomic.LoadInt32(&attempts))
		})
	}
}

// TestSendPostRequestShutdownPrioritizesFailure ensures shutdown always wins
// when it is already closed before sendPostRequest is called.
func TestSendPostRequestShutdownPrioritizesFailure(t *testing.T) {
	client := &Client{
		sendPostChan: make(chan *jsonRequest, 1),
		shutdown:     make(chan struct{}),
	}

	close(client.shutdown)

	const attempts = 200
	// The old single-select implementation chose randomly when both channels
	// were ready, so repeat enough times to make an accidental enqueue show up.
	for i := 0; i < attempts; i++ {
		jReq := &jsonRequest{
			id:           uint64(i),
			method:       "getblockcount",
			responseChan: make(chan *Response, 1),
		}
		client.sendPostRequest(jReq)

		select {
		case resp := <-jReq.responseChan:
			require.ErrorIs(t, resp.err, ErrClientShutdown)
		default:
			t.Fatalf("request id=%d was not failed immediately",
				jReq.id)
		}

		select {
		case <-client.sendPostChan:
			t.Fatalf("request id=%d was enqueued after shutdown",
				jReq.id)

		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestBatchSendErrorResolvesQueuedFutures ensures a batch send failure resolves
// all queued futures instead of leaving them blocked.
func TestBatchSendErrorResolvesQueuedFutures(t *testing.T) {
	connCfg := &ConnConfig{
		Host:         "127.0.0.1:8332",
		User:         "user",
		Pass:         "pass",
		DisableTLS:   true,
		HTTPPostMode: true,
	}

	client, err := NewBatch(connCfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Shutdown()
		client.WaitForShutdown()
	})

	client.httpClient.Transport = postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader("not-json"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		},
	)

	f1 := client.GetBlockCountAsync()
	f2 := client.GetBlockCountAsync()

	sendErr := client.Send()
	require.Error(t, sendErr)

	assertFutureErr := func(f FutureGetBlockCountResult) {
		t.Helper()

		done := make(chan error, 1)
		// Receive is the blocking caller-facing path. The old bug surfaced here
		// by never resolving the future, so bound it with a timeout.
		go func() {
			_, err := f.Receive()
			done <- err
		}()

		select {
		case err := <-done:
			require.Error(t, err)
			require.EqualError(t, err, sendErr.Error())

		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for queued batch future " +
				"to resolve")
		}
	}

	assertFutureErr(f1)
	assertFutureErr(f2)
}

// TestNewBatchSerializesPostSends ensures a batch client still serializes POST
// sends through a single handler goroutine.
func TestNewBatchSerializesPostSends(t *testing.T) {
	connCfg := &ConnConfig{
		Host:         "127.0.0.1:8332",
		User:         "user",
		Pass:         "pass",
		DisableTLS:   true,
		HTTPPostMode: true,
	}

	client, err := NewBatch(connCfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Shutdown()
		client.WaitForShutdown()
	})

	var active int32
	var maxActive int32
	release := make(chan struct{})

	client.httpClient.Transport = postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if current <= prev {
					break
				}
				if atomic.CompareAndSwapInt32(
					&maxActive, prev, current,
				) {
					break
				}
			}

			// Hold the request open so the test can observe if
			// a second POST enters the transport concurrently.
			<-release
			atomic.AddInt32(&active, -1)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"result":1,"error":null}`,
				)),
			}, nil
		},
	)

	makeReq := func(id uint64) *jsonRequest {
		return &jsonRequest{
			id:     id,
			method: "getblockcount",
			marshalledJSON: []byte(
				`{"jsonrpc":"1.0","id":1,` +
					`"method":"getblockcount","params":[]}`,
			),
			responseChan: make(chan *Response, 1),
		}
	}

	req1 := makeReq(1)
	req2 := makeReq(2)
	client.sendPostChan <- req1
	client.sendPostChan <- req2

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&active) >= 1
	}, time.Second, 5*time.Millisecond)

	// Allow any extra send handler goroutines to start a second in-flight
	// request.
	time.Sleep(100 * time.Millisecond)
	observedMax := atomic.LoadInt32(&maxActive)
	close(release)

	for i, req := range []*jsonRequest{req1, req2} {
		select {
		case resp := <-req.responseChan:
			require.NoError(t, resp.err, "request %d failed", i)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for request %d response", i)
		}
	}

	require.EqualValues(t, 1, observedMax, "POST sends must be serialized")
}

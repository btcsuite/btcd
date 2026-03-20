package rpcclient

import (
	"context"
	"errors"
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
}

// Read blocks until the request context is canceled, then returns that error.
func (b *cancelOnReadBody) Read(_ []byte) (int, error) {
	select {
	case <-b.readStarted:
	default:
		close(b.readStarted)
	}

	<-b.ctx.Done()

	return 0, b.ctx.Err()
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

// TestHandleSendPostMessageWithRetrySuccess ensures that
// handleSendPostMessageWithRetry returns a decoded result and no error on
// a successful response.
func TestHandleSendPostMessageWithRetrySuccess(t *testing.T) {
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

	result, err := handleSendPostMessageWithRetry(
		context.Background(), jReq, 1, client.httpClient, client.config,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []byte("1"), result)
}

// TestHandleSendPostMessageWithRetryShutdownDuringRetryBackoff ensures
// that handleSendPostMessageWithRetry returns context cancellation from the
// retry-backoff path.
func TestHandleSendPostMessageWithRetryShutdownDuringRetryBackoff(
	t *testing.T) {

	var attempts int32
	ctx, cancel := context.WithCancelCause(context.Background())

	client := newPostModeTestClient(postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			if atomic.AddInt32(&attempts, 1) == 1 {
				cancel(ErrClientShutdown)
			}
			return nil, errors.New("transient transport error")
		},
	))
	jReq := newPostTestRequest()

	result, err := handleSendPostMessageWithRetry(
		ctx, jReq, 2, client.httpClient, client.config, false,
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, atomic.LoadInt32(&attempts))
}

// TestHandleSendPostMessageWithRetryShutdownOnFinalRetry ensures that
// handleSendPostMessageWithRetry returns context cancellation from the final
// retry attempt path.
func TestHandleSendPostMessageWithRetryShutdownOnFinalRetry(t *testing.T) {
	var attempts int32
	ctx, cancel := context.WithCancelCause(context.Background())

	client := newPostModeTestClient(postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			current := atomic.AddInt32(&attempts, 1)
			if current == 1 {
				return nil, errors.New("transient transport " +
					"error")
			}

			cancel(ErrClientShutdown)
			return nil, context.Canceled
		},
	))
	jReq := newPostTestRequest()

	result, err := handleSendPostMessageWithRetry(
		ctx, jReq, 2, client.httpClient, client.config, false,
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 2, atomic.LoadInt32(&attempts))
}

// TestHandleSendPostMessageWithRetryShutdownOnBodyRead ensures that
// handleSendPostMessageWithRetry returns the body read error when cancellation
// arrives during io.ReadAll.
func TestHandleSendPostMessageWithRetryShutdownOnBodyRead(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	readStarted := make(chan struct{})

	client := newPostModeTestClient(postRoundTripFunc(
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: &cancelOnReadBody{
					ctx:         req.Context(),
					readStarted: readStarted,
				},
			}, nil
		},
	))
	jReq := newPostTestRequest()

	go func() {
		<-readStarted
		cancel(ErrClientShutdown)
	}()

	result, err := handleSendPostMessageWithRetry(
		ctx, jReq, 1, client.httpClient, client.config, false,
	)
	require.Nil(t, result)
	require.ErrorContains(t, err, "error reading json reply")
	require.ErrorIs(t, err, context.Canceled)
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

// TestHandleSendPostMessageShutdownDuringRetryBackoff ensures shutdown
// cancellation interrupts retry backoff and remaps to ErrClientShutdown.
func TestHandleSendPostMessageShutdownDuringRetryBackoff(t *testing.T) {
	var attempts int32
	attemptStarted := make(chan struct{}, 1)
	client := newPostModeTestClient(postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			atomic.AddInt32(&attempts, 1)
			attemptStarted <- struct{}{}
			return nil, errors.New("transient transport error")
		},
	))

	ctx, cancel := context.WithCancelCause(context.Background())
	jReq := newPostTestRequest()

	go client.handleSendPostMessageWithRetry(ctx, jReq, 2)

	select {
	case <-attemptStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first attempt did not start")
	}

	cancel(ErrClientShutdown)

	select {
	case resp := <-jReq.responseChan:
		require.ErrorIs(t, resp.err, ErrClientShutdown)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}
}

// TestHandleSendPostMessageShutdownOnFinalRetry ensures cancellation on the
// final retry attempt is still remapped to ErrClientShutdown.
func TestHandleSendPostMessageShutdownOnFinalRetry(t *testing.T) {
	var attempts int32
	ctx, cancel := context.WithCancelCause(context.Background())

	client := newPostModeTestClient(postRoundTripFunc(
		func(*http.Request) (*http.Response, error) {
			current := atomic.AddInt32(&attempts, 1)
			if current == 1 {
				return nil, errors.New("transient transport " +
					"error")
			}

			cancel(ErrClientShutdown)
			return nil, context.Canceled
		},
	))
	jReq := newPostTestRequest()

	go client.handleSendPostMessageWithRetry(ctx, jReq, 2)

	select {
	case resp := <-jReq.responseChan:
		require.ErrorIs(t, resp.err, ErrClientShutdown)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}

	require.EqualValues(t, 2, atomic.LoadInt32(&attempts))
}

// TestHandleSendPostMessageShutdownDuringBodyRead ensures cancellation while
// reading a response body is remapped to ErrClientShutdown.
func TestHandleSendPostMessageShutdownDuringBodyRead(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	readStarted := make(chan struct{})

	client := newPostModeTestClient(postRoundTripFunc(
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: &cancelOnReadBody{
					ctx:         req.Context(),
					readStarted: readStarted,
				},
			}, nil
		},
	))
	jReq := newPostTestRequest()

	go client.handleSendPostMessageWithRetry(ctx, jReq, 1)

	select {
	case <-readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("response body read did not start")
	}

	cancel(ErrClientShutdown)

	select {
	case resp := <-jReq.responseChan:
		require.ErrorIs(t, resp.err, ErrClientShutdown)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
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

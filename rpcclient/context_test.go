package rpcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/stretchr/testify/require"
)

// TestGetBlockCountWithContextSuccess verifies that the context-
// aware GetBlockCountWithContext returns the correct block height.
func TestGetBlockCountWithContextSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":941874,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	count, err := client.GetBlockCountWithContext(
		context.Background(),
	)
	require.NoError(t, err)
	require.Equal(t, int64(941874), count)
}

// TestGetBlockCountWithContextCancelledContext verifies that a
// cancelled context stops the HTTP request and returns a context
// cancellation error.
func TestGetBlockCountWithContextCancelledContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow RPC — delay longer than the
			// context timeout so cancellation is tested.
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	ctx, cancel := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	_, err := client.GetBlockCountWithContext(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(
		t, elapsed, 2*time.Second,
		"context cancellation should abort the request",
	)
}

// TestSendCmdWithContextRetryRespectsCancel verifies that context
// cancellation during the retry backoff terminates immediately
// rather than waiting for all retry attempts.
func TestSendCmdWithContextRetryRespectsCancel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Fatal("request should be intercepted by custom transport")
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	var attempts int
	client.httpClient.Transport = roundTripperFunc(
		func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, errors.New("boom")
		},
	)

	ctx, cancel := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	_, err := client.GetBlockCountWithContext(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, attempts)
	require.Less(
		t, elapsed, 2*time.Second,
		"context cancellation should stop retries promptly",
	)
}

// TestGetBlockCountBackwardsCompatible verifies that the original
// GetBlockCount (without context) still works unchanged.
func TestGetBlockCountBackwardsCompatible(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":100,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	count, err := client.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, int64(100), count)
}

// TestHandleSendPostMessageNilCtxFallsBackToBackground verifies
// that internal callers which leave jsonRequest.ctx nil still work
// through the HTTP POST path.
func TestHandleSendPostMessageNilCtxFallsBackToBackground(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":500,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	cmd := btcjson.NewGetBlockCountCmd()
	marshalledJSON, err := btcjson.MarshalCmd(
		btcjson.RpcVersion1, 1, cmd,
	)
	require.NoError(t, err)

	responseChan := make(chan *Response, 1)
	jReq := &jsonRequest{
		id:             1,
		method:         "getblockcount",
		cmd:            cmd,
		marshalledJSON: marshalledJSON,
		responseChan:   responseChan,
	}

	client.handleSendPostMessage(jReq)

	count, err := FutureGetBlockCountResult(responseChan).Receive()
	require.NoError(t, err)
	require.Equal(t, int64(500), count)
}

// TestGetBlockCountWithContextRPCError verifies that JSON-RPC
// errors are properly propagated through the context path.
func TestGetBlockCountWithContextRPCError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := map[string]interface{}{
				"result": nil,
				"error": map[string]interface{}{
					"code":    -1,
					"message": "method not found",
				},
				"id": 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	_, err := client.GetBlockCountWithContext(
		context.Background(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "method not found")
}

// newTestHTTPClient creates an rpcclient connected to the given
// httptest.Server in HTTP POST mode.
func newTestHTTPClient(
	t *testing.T, server *httptest.Server,
) *Client {

	t.Helper()

	// Strip "http://" prefix for ConnConfig.Host.
	host := strings.TrimPrefix(server.URL, "http://")

	client, err := New(&ConnConfig{
		Host:         host,
		User:         "user",
		Pass:         "pass",
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	require.NoError(t, err)

	return client
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(
	r *http.Request,
) (*http.Response, error) {

	return f(r)
}

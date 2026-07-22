package rpcclient

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

const (
	testRPCUser     = "testuser"
	testRPCPass     = "testpass"
	testCallerAuth  = "Bearer test-api-key"
	testExtraHeader = "X-Test-API-Key"
	testExtraValue  = "test-api-key"
)

// disableAuthTestCase describes one authentication header configuration that
// must behave the same for HTTP POST and WebSocket transports.
type disableAuthTestCase struct {
	name              string
	configure         func(*ConnConfig)
	wantAuthorization string
}

// disableAuthTestCases returns the shared transport authentication cases.
func disableAuthTestCases(missingCookie string) []disableAuthTestCase {
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString(
		[]byte(testRPCUser+":"+testRPCPass),
	)

	return []disableAuthTestCase{
		{
			name: "disabled omits generated authorization",
			configure: func(config *ConnConfig) {
				config.User = ""
				config.Pass = ""
				config.CookiePath = missingCookie
				config.DisableAuth = true
			},
		},
		{
			name: "disabled preserves caller authorization",
			configure: func(config *ConnConfig) {
				config.User = ""
				config.Pass = ""
				config.CookiePath = missingCookie
				config.DisableAuth = true
				config.ExtraHeaders["Authorization"] =
					testCallerAuth
			},
			wantAuthorization: testCallerAuth,
		},
		{
			name: "explicit false includes basic authorization",
			configure: func(config *ConnConfig) {
				config.DisableAuth = false
			},
			wantAuthorization: basicAuth,
		},
		{
			name: "zero value includes basic authorization",
			configure: func(*ConnConfig) {
				// Leave DisableAuth at its zero value.
			},
			wantAuthorization: basicAuth,
		},
	}
}

// newDisableAuthConfig creates the common configuration for the transport
// authentication cases.
func newDisableAuthConfig() *ConnConfig {
	return &ConnConfig{
		User: testRPCUser,
		Pass: testRPCPass,
		ExtraHeaders: map[string]string{
			testExtraHeader: testExtraValue,
		},
	}
}

// assertAuthHeaders verifies both generated or caller-supplied authorization
// and the independent extra header.
func assertAuthHeaders(t *testing.T, header http.Header,
	wantAuthorization string) {

	t.Helper()

	require.Equal(t, wantAuthorization, header.Get("Authorization"))
	require.Equal(t, testExtraValue, header.Get(testExtraHeader))
}

// TestDisableAuthHTTPPost verifies that DisableAuth controls generated Basic
// Auth headers on HTTP POST requests without suppressing caller headers.
func TestDisableAuthHTTPPost(t *testing.T) {
	missingCookie := filepath.Join(t.TempDir(), "missing-cookie")

	for _, tc := range disableAuthTestCases(missingCookie) {
		t.Run(tc.name, func(t *testing.T) {
			requestHeader := make(chan http.Header, 1)
			client := newPostModeTestClient(postRoundTripFunc(
				func(req *http.Request) (*http.Response, error) {
					requestHeader <- req.Header.Clone()

					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body: io.NopCloser(strings.NewReader(
							`{"result":1,"error":null,"id":1}`,
						)),
					}, nil
				},
			))
			client.config = newDisableAuthConfig()
			client.config.Host = "127.0.0.1:8332"
			client.config.DisableTLS = true
			client.config.HTTPPostMode = true
			tc.configure(client.config)

			result, err := sendPostRequestWithRetry(
				context.Background(), newPostTestRequest(), 1,
				client.httpClient, client.config, client.httpURL,
				false,
			)
			require.NoError(t, err)
			require.Equal(t, []byte("1"), result)

			select {
			case header := <-requestHeader:
				assertAuthHeaders(t, header, tc.wantAuthorization)

			case <-time.After(time.Second):
				t.Fatal("timed out waiting for HTTP POST request")
			}
		})
	}
}

// newWebsocketAuthServer creates a server that records the WebSocket handshake
// headers before upgrading the connection.
func newWebsocketAuthServer(t *testing.T) (string, <-chan http.Header) {
	t.Helper()

	requestHeader := make(chan http.Header, 1)
	upgrader := websocket.Upgrader{}
	handler := http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			requestHeader <- req.Header.Clone()

			conn, err := upgrader.Upgrade(w, req, nil)
			if err != nil {
				return
			}
			defer func() {
				_ = conn.Close()
			}()
		},
	)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://"), requestHeader
}

// TestDisableAuthWebsocket verifies that DisableAuth controls generated Basic
// Auth headers on WebSocket handshakes without suppressing caller headers.
func TestDisableAuthWebsocket(t *testing.T) {
	missingCookie := filepath.Join(t.TempDir(), "missing-cookie")

	for _, tc := range disableAuthTestCases(missingCookie) {
		t.Run(tc.name, func(t *testing.T) {
			host, requestHeader := newWebsocketAuthServer(t)
			config := newDisableAuthConfig()
			config.Host = host
			config.DisableTLS = true
			tc.configure(config)

			conn, err := dial(config)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, conn.Close())
			})

			select {
			case header := <-requestHeader:
				assertAuthHeaders(t, header, tc.wantAuthorization)

			case <-time.After(time.Second):
				t.Fatal("timed out waiting for WebSocket handshake")
			}
		})
	}
}

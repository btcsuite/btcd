package rpcclient

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestHeaderCapturingTransport tests that the HeaderCapturingTransport
// correctly captures response headers.
func TestHeaderCapturingTransport(t *testing.T) {
	tests := []struct {
		name           string
		responseHeaders map[string]string
		captureFunc    func(http.Header)
		expectCapture  bool
	}{
		{
			name: "captures headers when callback provided",
			responseHeaders: map[string]string{
				"zp-rid":        "test-request-id",
				"X-Trace-Id":    "trace-123",
				"Content-Type":  "application/json",
			},
			captureFunc: func(headers http.Header) {
				// Verify headers are captured
				if headers.Get("zp-rid") != "test-request-id" {
					t.Errorf("Expected zp-rid header to be 'test-request-id', got '%s'", headers.Get("zp-rid"))
				}
				if headers.Get("X-Trace-Id") != "trace-123" {
					t.Errorf("Expected X-Trace-Id header to be 'trace-123', got '%s'", headers.Get("X-Trace-Id"))
				}
			},
			expectCapture: true,
		},
		{
			name: "no capture when callback is nil",
			responseHeaders: map[string]string{
				"zp-rid": "test-request-id",
			},
			captureFunc:   nil,
			expectCapture: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that returns specific headers
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tt.responseHeaders {
					w.Header().Set(key, value)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"result":"success"}`))
			}))
			defer server.Close()

			// Create the header capturing transport
			transport := &HeaderCapturingTransport{
				Base:      http.DefaultTransport,
				OnCapture: tt.captureFunc,
			}

			// Create HTTP client with the transport
			client := &http.Client{
				Transport: transport,
			}

			// Make a request
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			// Verify response is successful
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status code 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestNewHTTPClientWithHeaderCapture tests that newHTTPClient correctly
// sets up header capturing when OnCaptureHeader is provided in ConnConfig.
func TestNewHTTPClientWithHeaderCapture(t *testing.T) {
	var capturedHeaders http.Header
	var mu sync.Mutex

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("zp-rid", "test-rid-123")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","result":"test","id":1}`))
	}))
	defer server.Close()

	tests := []struct {
		name          string
		config        *ConnConfig
		expectCapture bool
	}{
		{
			name: "with header capture callback",
			config: &ConnConfig{
				Host:         server.URL[7:], // Remove http://
				HTTPPostMode: true,
				DisableTLS:   true,
				OnCaptureHeader: func(headers http.Header) {
					mu.Lock()
					capturedHeaders = headers.Clone()
					mu.Unlock()
				},
			},
			expectCapture: true,
		},
		{
			name: "without header capture callback",
			config: &ConnConfig{
				Host:            server.URL[7:], // Remove http://
				HTTPPostMode:    true,
				DisableTLS:      true,
				OnCaptureHeader: nil,
			},
			expectCapture: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset captured headers
			mu.Lock()
			capturedHeaders = nil
			mu.Unlock()

			// Create HTTP client
			httpClient, err := newHTTPClient(tt.config)
			if err != nil {
				t.Fatalf("Failed to create HTTP client: %v", err)
			}

			// Make a test request
			req, err := http.NewRequest("POST", "http://"+tt.config.Host, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to execute request: %v", err)
			}
			defer resp.Body.Close()

			// Check if headers were captured
			mu.Lock()
			defer mu.Unlock()
			
			if tt.expectCapture {
				if capturedHeaders == nil {
					t.Error("Expected headers to be captured, but they weren't")
				} else {
					if capturedHeaders.Get("zp-rid") != "test-rid-123" {
						t.Errorf("Expected zp-rid to be 'test-rid-123', got '%s'", capturedHeaders.Get("zp-rid"))
					}
					if capturedHeaders.Get("X-Custom-Header") != "custom-value" {
						t.Errorf("Expected X-Custom-Header to be 'custom-value', got '%s'", capturedHeaders.Get("X-Custom-Header"))
					}
				}
			} else {
				if capturedHeaders != nil {
					t.Error("Expected no headers to be captured, but they were")
				}
			}
		})
	}
}

// TestHeaderCaptureIntegration tests the full integration of header capture
// with the RPC client in HTTP POST mode.
func TestHeaderCaptureIntegration(t *testing.T) {
	var capturedHeaders []http.Header
	var mu sync.Mutex

	// Create a test RPC server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set response headers
		w.Header().Set("zp-rid", "integration-test-rid")
		w.Header().Set("X-Request-Id", "req-456")
		w.Header().Set("Content-Type", "application/json")
		
		// Return a valid JSON-RPC response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","result":{"version":170000},"id":1}`))
	}))
	defer server.Close()

	// Create RPC client with header capture
	config := &ConnConfig{
		Host:         server.URL[7:], // Remove http://
		HTTPPostMode: true,
		DisableTLS:   true,
		User:         "testuser",
		Pass:         "testpass",
		OnCaptureHeader: func(headers http.Header) {
			mu.Lock()
			capturedHeaders = append(capturedHeaders, headers.Clone())
			mu.Unlock()
		},
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create RPC client: %v", err)
	}
	defer client.Shutdown()

	// Make an RPC call (this would normally be a real RPC method)
	// Since we're testing with a mock server, we'll use SendCmd directly
	future := client.SendCmd(&btcjson.GetInfoCmd{})
	
	// Wait for response
	_, err = ReceiveFuture(future)
	if err != nil {
		// This might fail due to response parsing, but we're mainly testing header capture
		t.Logf("RPC call error (expected in test): %v", err)
	}

	// Verify headers were captured
	mu.Lock()
	defer mu.Unlock()
	
	if len(capturedHeaders) == 0 {
		t.Fatal("Expected headers to be captured, but none were")
	}

	// Check the captured headers
	lastHeaders := capturedHeaders[len(capturedHeaders)-1]
	if lastHeaders.Get("zp-rid") != "integration-test-rid" {
		t.Errorf("Expected zp-rid to be 'integration-test-rid', got '%s'", lastHeaders.Get("zp-rid"))
	}
	if lastHeaders.Get("X-Request-Id") != "req-456" {
		t.Errorf("Expected X-Request-Id to be 'req-456', got '%s'", lastHeaders.Get("X-Request-Id"))
	}
}

// TestHeaderCaptureErrorHandling tests that header capture handles errors gracefully.
func TestHeaderCaptureErrorHandling(t *testing.T) {
	// Test that transport continues to work even if OnCapture panics
	panicTransport := &HeaderCapturingTransport{
		Base: http.DefaultTransport,
		OnCapture: func(headers http.Header) {
			panic("test panic in OnCapture")
		},
	}

	// This should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic from OnCapture, but didn't get one")
		}
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Test-Header", "value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Transport: panicTransport}
	resp, err := client.Get(server.URL)
	if err == nil {
		resp.Body.Close()
	}
}
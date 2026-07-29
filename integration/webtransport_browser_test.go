//go:build rpctest
// +build rpctest

// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

const (
	webTransportTestPath      = "/v1/btc-p2p"
	webTransportTestUserAgent = wire.DefaultUserAgent +
		"btcd-wasm-itest:0.0.1/"
)

type webTransportBrowserResult struct {
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	ServerUserAgent string `json:"server_user_agent,omitempty"`
	ServerProtocol  int32  `json:"server_protocol,omitempty"`
	SendAddrV2      bool   `json:"send_addr_v2,omitempty"`
	Pong            bool   `json:"pong,omitempty"`
	Transport       string `json:"transport,omitempty"`
}

type chromeProcess struct {
	cancel  context.CancelFunc
	done    chan struct{}
	logPath string
	errMu   sync.Mutex
	err     error
	once    sync.Once
}

// TestWebTransportBrowserWASMPeer proves the complete browser-facing path:
// btcd's peer package runs in Go WASM, dials through the browser WebTransport
// API with a pinned certificate hash, and completes the Bitcoin handshake with
// a full native btcd process.  The native node's RPC view must classify the
// resulting peer as inbound on the WebTransport listener.
func TestWebTransportBrowserWASMPeer(t *testing.T) {
	chromePath := findChrome()
	manualBrowser := os.Getenv("BTCD_WEBTRANSPORT_BROWSER_MANUAL") == "1"
	if chromePath == "" && !manualBrowser {
		t.Skip("Chrome or Chromium not found; set BTCD_CHROME_BIN to run " +
			"the WebTransport browser integration test")
	}

	repoRoot, fixtureDir := webTransportFixturePaths(t)
	testDir := t.TempDir()
	wasmPath := filepath.Join(testDir, "client.wasm")
	buildWASMFixture(t, repoRoot, wasmPath)
	wasmExec := readWASMExec(t)
	indexHTML, err := os.ReadFile(filepath.Join(fixtureDir, "index.html"))
	require.NoError(t, err)
	wasmBinary, err := os.ReadFile(wasmPath)
	require.NoError(t, err)

	resultChan := make(chan webTransportBrowserResult, 1)
	browserListener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	browserOrigin := "http://" + browserListener.Addr().String()
	browserServer := newWebTransportAssetServer(
		indexHTML, wasmExec, wasmBinary, resultChan,
	)
	serveError := make(chan error, 1)
	go func() {
		serveError <- browserServer.Serve(browserListener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()
		if err := browserServer.Shutdown(ctx); err != nil {
			t.Errorf("shut down browser asset server: %v", err)
		}
		select {
		case err := <-serveError:
			if err != nil && err != http.ErrServerClosed {
				t.Errorf("browser asset server: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("browser asset server did not stop")
		}
	})

	webTransportAddress := availableUDPAddress(t)
	certificatePath := filepath.Join(testDir, "webtransport.cert")
	keyPath := filepath.Join(testDir, "webtransport.key")
	certificateHash := writeWebTransportCertificate(
		t, certificatePath, keyPath,
	)

	harness, err := rpctest.New(
		&chaincfg.SimNetParams, nil, []string{
			"--webtransportlisten=" + webTransportAddress,
			"--webtransportcert=" + certificatePath,
			"--webtransportkey=" + keyPath,
			"--webtransportpath=" + webTransportTestPath,
			"--webtransportorigin=" + browserOrigin,
		}, "",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := harness.TearDown(); err != nil {
			t.Errorf("tear down btcd harness: %v", err)
		}
	})
	require.NoError(t, harness.SetUp(false, 0))

	endpoint := "https://" + webTransportAddress + webTransportTestPath
	query := make(url.Values)
	query.Set("endpoint", endpoint)
	query.Set("cert_hash", fmt.Sprintf("%x", certificateHash[:]))
	pageURL := browserOrigin + "/?" + query.Encode()

	var chrome *chromeProcess
	if manualBrowser {
		t.Logf("manual WebTransport browser URL: %s", pageURL)
	} else {
		version, err := exec.Command(chromePath, "--version").CombinedOutput()
		if err != nil {
			t.Logf("unable to read Chrome version: %v", err)
		} else {
			t.Logf("browser: %s", strings.TrimSpace(string(version)))
		}

		chrome = startChrome(t, chromePath, pageURL, testDir)
		t.Cleanup(func() {
			chrome.stop()
		})
	}

	resultWait := 45 * time.Second
	if manualBrowser {
		resultWait = 5 * time.Minute
	}
	resultTimeout := time.NewTimer(resultWait)
	defer resultTimeout.Stop()

	var result webTransportBrowserResult
	select {
	case result = <-resultChan:
	case <-chromeDone(chrome):
		t.Fatalf("Chrome exited before reporting a result: %v\n%s",
			chrome.exitError(), chrome.logOutput())
	case <-resultTimeout.C:
		if chrome == nil {
			t.Fatal("timed out waiting for the manual browser result")
		}
		chrome.stop()
		t.Fatalf("timed out waiting for Chrome result\n%s",
			chrome.logOutput())
	}

	if result.Status != "ok" {
		if chrome != nil {
			chrome.stop()
		}
		t.Fatalf("browser WebTransport client failed: %s\n%s",
			result.Error, chromeLogOutput(chrome))
	}
	require.Contains(t, result.ServerUserAgent, "/btcd:")
	require.Equal(t, int32(wire.ProtocolVersion), result.ServerProtocol)
	require.True(t, result.SendAddrV2)
	require.True(t, result.Pong)
	require.Equal(t, "webtransport", result.Transport)

	var rpcPeerFound bool
	require.Eventually(t, func() bool {
		peers, err := harness.Client.GetPeerInfo()
		if err != nil {
			return false
		}
		for _, p := range peers {
			if p.SubVer != webTransportTestUserAgent {
				continue
			}
			rpcPeerFound = p.Inbound &&
				p.AddrLocal == webTransportAddress &&
				p.Version == uint32(result.ServerProtocol) &&
				p.BytesRecv > 0 && p.BytesSent > 0
			return rpcPeerFound
		}
		return false
	}, 10*time.Second, 50*time.Millisecond)
	require.True(t, rpcPeerFound)

	t.Logf("browser WebTransport peer completed version/sendaddrv2/"+
		"verack and ping/pong; btcd RPC reports inbound=true, "+
		"addrlocal=%s", webTransportAddress)
}

func webTransportFixturePaths(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	integrationDir := filepath.Dir(file)
	return filepath.Dir(integrationDir), filepath.Join(
		integrationDir, "testdata", "webtransport_wasm",
	)
}

func buildWASMFixture(t *testing.T, repoRoot, outputPath string) {
	t.Helper()
	cmd := exec.Command(
		"go", "build", "-trimpath", "-o", outputPath,
		"./integration/testdata/webtransport_wasm",
	)
	cmd.Dir = repoRoot
	cmd.Env = append(
		os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0",
	)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "build Go WASM fixture:\n%s", output)
}

func readWASMExec(t *testing.T) []byte {
	t.Helper()
	paths := []string{
		filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js"),
		filepath.Join(runtime.GOROOT(), "misc", "wasm", "wasm_exec.js"),
	}
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err == nil {
			return contents
		}
	}
	t.Fatalf("wasm_exec.js not found below %s", runtime.GOROOT())
	return nil
}

func newWebTransportAssetServer(indexHTML, wasmExec, wasmBinary []byte,
	resultChan chan<- webTransportBrowserResult) *http.Server {

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter,
		_ *http.Request) {

		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(wasmExec)
	})
	mux.HandleFunc("/client.wasm", func(w http.ResponseWriter,
		_ *http.Request) {

		w.Header().Set("Content-Type", "application/wasm")
		_, _ = w.Write(wasmBinary)
	})
	mux.HandleFunc("/result", func(w http.ResponseWriter,
		request *http.Request) {

		if request.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		defer request.Body.Close()
		decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
		var result webTransportBrowserResult
		if err := decoder.Decode(&result); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case resultChan <- result:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "result already reported", http.StatusConflict)
		}
	})

	return &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func availableUDPAddress(t *testing.T) string {
	t.Helper()
	packetConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	address := packetConn.LocalAddr().String()
	require.NoError(t, packetConn.Close())
	return address
}

func writeWebTransportCertificate(t *testing.T, certPath,
	keyPath string) [sha256.Size]byte {

	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "btcd WebTransport test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(
		rand.Reader, template, template, &privateKey.PublicKey, privateKey,
	)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: der,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "EC PRIVATE KEY", Bytes: keyDER,
	})
	require.NoError(t, os.WriteFile(certPath, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))
	return sha256.Sum256(der)
}

func findChrome() string {
	if configured := os.Getenv("BTCD_CHROME_BIN"); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}

	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium",
		"chromium-browser", "chrome",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func startChrome(t *testing.T, chromePath, pageURL, testDir string) *chromeProcess {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	args := []string{
		"--headless=new",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--disable-sync",
		"--metrics-recording-only",
		"--no-default-browser-check",
		"--no-first-run",
		"--user-data-dir=" + filepath.Join(testDir, "chrome-profile"),
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--no-sandbox")
	}
	args = append(args, pageURL)

	logPath := filepath.Join(testDir, "chrome.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, chromePath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())

	process := &chromeProcess{
		cancel:  cancel,
		done:    make(chan struct{}),
		logPath: logPath,
	}
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		process.errMu.Lock()
		process.err = err
		process.errMu.Unlock()
		close(process.done)
	}()
	return process
}

func (p *chromeProcess) stop() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
		}
	})
}

func (p *chromeProcess) logOutput() string {
	if p == nil {
		return ""
	}
	contents, err := os.ReadFile(p.logPath)
	if err != nil {
		return fmt.Sprintf("read Chrome log: %v", err)
	}
	const maxLogBytes = 16 << 10
	if len(contents) > maxLogBytes {
		contents = contents[len(contents)-maxLogBytes:]
	}
	return string(contents)
}

func (p *chromeProcess) exitError() error {
	if p == nil {
		return nil
	}
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}

func chromeDone(chrome *chromeProcess) <-chan struct{} {
	if chrome == nil {
		return make(chan struct{})
	}
	return chrome.done
}

func chromeLogOutput(chrome *chromeProcess) string {
	if chrome == nil {
		return ""
	}
	return chrome.logOutput()
}

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeWebTransportAddresses(t *testing.T) {
	t.Parallel()

	addresses, err := normalizeWebTransportAddresses([]string{
		"127.0.0.1:4433", "[::1]:4433", "127.0.0.1:4433",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"127.0.0.1:4433", "[::1]:4433"}, addresses)

	for _, address := range []string{"127.0.0.1", "127.0.0.1:0", ":https"} {
		_, err := normalizeWebTransportAddresses([]string{address})
		require.Error(t, err)
	}
}

func TestValidateWebTransportPath(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateWebTransportPath("/v1/btc-p2p"))
	for _, path := range []string{"", "v1/p2p", "//host/p2p", "/p2p?x=1", "/p2p#x"} {
		require.Error(t, validateWebTransportPath(path), path)
	}
}

func TestConfigureP2PListeners(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cfg           config
		wantListeners []string
		wantErr       string
	}{
		{
			name:          "ordinary default TCP listener",
			wantListeners: []string{":8333"},
		},
		{
			name: "WebTransport adds to default TCP",
			cfg: config{
				WebTransportListen: []string{"127.0.0.1:4433"},
			},
			wantListeners: []string{":8333"},
		},
		{
			name: "explicit WebTransport only",
			cfg: config{
				DisableTCPListen:   true,
				WebTransportListen: []string{"127.0.0.1:4433"},
			},
		},
		{
			name: "nolisten overrides WebTransport",
			cfg: config{
				DisableListen:      true,
				WebTransportListen: []string{"127.0.0.1:4433"},
			},
		},
		{
			name: "TCP disabled without WebTransport",
			cfg: config{
				DisableTCPListen: true,
			},
			wantErr: "requires --webtransportlisten",
		},
		{
			name: "TCP listener conflicts with disabled TCP",
			cfg: config{
				DisableTCPListen: true,
				Listeners:        []string{"127.0.0.1:8333"},
				WebTransportListen: []string{
					"127.0.0.1:4433",
				},
			},
			wantErr: "can not be mixed",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := configureP2PListeners(&test.cfg, "8333")
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantListeners, test.cfg.Listeners)
		})
	}
}

func TestValidateMaxPeers(t *testing.T) {
	tests := []struct {
		name     string
		maxPeers int
		wantErr  bool
	}{
		{name: "negative", maxPeers: -1, wantErr: true},
		{name: "zero", wantErr: true},
		{name: "positive", maxPeers: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMaxPeers(test.maxPeers)
			if test.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

var (
	rpcuserRegexp = regexp.MustCompile("(?m)^rpcuser=.+$")
	rpcpassRegexp = regexp.MustCompile("(?m)^rpcpass=.+$")
)

func TestCreateDefaultConfigFile(t *testing.T) {
	// find out where the sample config lives
	_, path, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed finding config file path")
	}
	sampleConfigFile := filepath.Join(filepath.Dir(path), "sample-btcd.conf")

	// Setup a temporary directory
	tmpDir := t.TempDir()
	testpath := filepath.Join(tmpDir, "test.conf")

	// copy config file to location of btcd binary
	data, err := os.ReadFile(sampleConfigFile)
	if err != nil {
		t.Fatalf("Failed reading sample config file: %v", err)
	}
	appPath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		t.Fatalf("Failed obtaining app path: %v", err)
	}
	tmpConfigFile := filepath.Join(appPath, "sample-btcd.conf")
	err = os.WriteFile(tmpConfigFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed copying sample config file: %v", err)
	}

	err = createDefaultConfigFile(testpath)

	if err != nil {
		t.Fatalf("Failed to create a default config file: %v", err)
	}

	content, err := os.ReadFile(testpath)
	if err != nil {
		t.Fatalf("Failed to read generated default config file: %v", err)
	}

	if !rpcuserRegexp.Match(content) {
		t.Error("Could not find rpcuser in generated default config file.")
	}

	if !rpcpassRegexp.Match(content) {
		t.Error("Could not find rpcpass in generated default config file.")
	}
}

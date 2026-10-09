package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitWebTransportListenersRejectsInvalidCertificate(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() {
		cfg = originalCfg
	})

	testDir := t.TempDir()
	certPath := filepath.Join(testDir, "webtransport.cert")
	keyPath := filepath.Join(testDir, "webtransport.key")
	require.NoError(t, os.WriteFile(certPath, []byte("invalid cert"), 0600))
	require.NoError(t, os.WriteFile(keyPath, []byte("invalid key"), 0600))

	cfg = &config{
		WebTransportCert: certPath,
		WebTransportKey:  keyPath,
	}

	listeners, err := initWebTransportListeners([]string{"127.0.0.1:0"})
	require.ErrorContains(t, err, "load WebTransport certificate")
	require.Empty(t, listeners)
}

func TestInitWebTransportListenersClosesEarlierListener(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() {
		cfg = originalCfg
	})

	testDir := t.TempDir()
	certPath := filepath.Join(testDir, "webtransport.cert")
	keyPath := filepath.Join(testDir, "webtransport.key")
	require.NoError(t, genCertPair(certPath, keyPath))

	cfg = &config{
		WebTransportCert: certPath,
		WebTransportKey:  keyPath,
	}

	blocked, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, blocked.Close())
	})

	probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	firstAddress := probe.LocalAddr().String()
	require.NoError(t, probe.Close())

	listeners, err := initWebTransportListeners([]string{
		firstAddress, blocked.LocalAddr().String(),
	})
	require.Error(t, err)
	require.Empty(t, listeners)

	rebound, err := net.ListenPacket("udp4", firstAddress)
	require.NoError(t, err, "first listener was not closed after later failure")
	require.NoError(t, rebound.Close())
}

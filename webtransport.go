package main

import (
	"crypto/tls"
	"fmt"
	"net"

	btcdwebtransport "github.com/btcsuite/btcd/internal/webtransport"
)

func initWebTransportListeners(addresses []string) ([]net.Listener, error) {
	if len(addresses) == 0 {
		return nil, nil
	}

	certificate, err := tls.LoadX509KeyPair(
		cfg.WebTransportCert, cfg.WebTransportKey,
	)
	if err != nil {
		return nil, fmt.Errorf("load WebTransport certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS13,
	}
	listeners := make([]net.Listener, 0, len(addresses))
	for _, address := range addresses {
		listener, err := btcdwebtransport.Listen(
			"udp", address, btcdwebtransport.Config{
				TLSConfig:      tlsConfig,
				Path:           cfg.WebTransportPath,
				AllowedOrigins: cfg.WebTransportOrigins,
			},
		)
		if err != nil {
			closeListeners(listeners)
			return nil, fmt.Errorf(
				"listen for WebTransport on %s: %w", address, err,
			)
		}

		listeners = append(listeners, listener)
	}

	return listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

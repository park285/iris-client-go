package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	envH3CACertFile   = "IRIS_H3_CA_CERT_FILE"
	envH3ServerName   = "IRIS_H3_SERVER_NAME"
	envH3InsecureSkip = "IRIS_H3_INSECURE_SKIP_VERIFY"
)

func newHTTP3Transport(opts clientOptions) (*http3.Transport, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	serverName := firstNonEmpty(opts.h3ServerName, os.Getenv(envH3ServerName))
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}

	caCertFile := firstNonEmpty(opts.h3CACertFile, os.Getenv(envH3CACertFile))
	if caCertFile != "" {
		pemBytes, err := os.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("read IRIS_H3_CA_CERT_FILE: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("parse IRIS_H3_CA_CERT_FILE")
		}

		tlsCfg.RootCAs = pool
	}

	if !opts.h3InsecureSkipVerify && opts.allowInsecureForTests {
		opts.h3InsecureSkipVerify = parseBoolEnv(os.Getenv(envH3InsecureSkip))
	}
	if opts.h3InsecureSkipVerify {
		if !opts.allowInsecureForTests {
			return nil, fmt.Errorf("IRIS_H3_INSECURE_SKIP_VERIFY is allowed only in tests/local mode")
		}

		tlsCfg.InsecureSkipVerify = true
	}

	return &http3.Transport{
		TLSClientConfig: tlsCfg,
		QUICConfig: &quic.Config{
			KeepAlivePeriod: 10 * time.Second,
			MaxIdleTimeout:  60 * time.Second,
		},
		Dial: dialHTTP3UDP4,
	}, nil
}

func dialHTTP3UDP4(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}

	packetConn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}

	conn, err := quic.Dial(ctx, packetConnNoOOB{PacketConn: packetConn}, udpAddr, tlsCfg, cfg)
	if err != nil {
		_ = packetConn.Close()
		return nil, err
	}

	return conn, nil
}

type packetConnNoOOB struct {
	net.PacketConn
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseBoolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

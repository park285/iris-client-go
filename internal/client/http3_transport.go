package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
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
			InitialPacketSize: 1200,
			KeepAlivePeriod:   10 * time.Second,
			MaxIdleTimeout:    60 * time.Second,
		},
	}, nil
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

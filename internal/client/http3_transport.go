package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	envH3CACertFile       = "IRIS_H3_CA_CERT_FILE"
	envH3ServerName       = "IRIS_H3_SERVER_NAME"
	envH3InsecureSkip     = "IRIS_H3_INSECURE_SKIP_VERIFY"
	envH3CAReloadInterval = "IRIS_H3_CA_RELOAD_INTERVAL"
	envH3AllowSystemRoots = "IRIS_H3_ALLOW_SYSTEM_ROOTS"
)

var (
	ErrEmptyH3CACertFile   = errors.New("iris: IRIS_H3_CA_CERT_FILE is set but empty; refusing to fall back to system roots")
	ErrMissingH3CACertFile = errors.New("iris: no H3 CA cert file configured; set IRIS_H3_CA_CERT_FILE or opt in with WithH3AllowSystemRoots/IRIS_H3_ALLOW_SYSTEM_ROOTS")
)

func resolveH3CACertFile(opts clientOptions) string {
	return firstNonEmpty(opts.h3CACertFile, os.Getenv(envH3CACertFile))
}

func resolveH3AllowSystemRoots(opts clientOptions) bool {
	return opts.h3AllowSystemRoots || parseBoolEnv(os.Getenv(envH3AllowSystemRoots))
}

func newHTTP3Transport(opts clientOptions) (*http3.Transport, error) {
	var pemBytes []byte
	caCertFile := resolveH3CACertFile(opts)
	if caCertFile != "" {
		b, err := os.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("read IRIS_H3_CA_CERT_FILE: %w", err)
		}
		pemBytes = b
	}

	return newHTTP3TransportFromCA(opts, caCertFile != "", pemBytes)
}

func newHTTP3TransportFromCA(opts clientOptions, caConfigured bool, pemBytes []byte) (*http3.Transport, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	serverName := firstNonEmpty(opts.h3ServerName, os.Getenv(envH3ServerName))
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}

	insecure := opts.h3InsecureSkipVerify && opts.allowInsecureForTests
	switch {
	case len(pemBytes) > 0:
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("parse IRIS_H3_CA_CERT_FILE")
		}
		tlsCfg.RootCAs = pool
	case insecure:
	case caConfigured:
		return nil, ErrEmptyH3CACertFile
	case !resolveH3AllowSystemRoots(opts):
		return nil, ErrMissingH3CACertFile
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

	transport := &http3.Transport{
		TLSClientConfig: tlsCfg,
		QUICConfig: &quic.Config{
			InitialPacketSize: 1200,
			KeepAlivePeriod:   10 * time.Second,
			MaxIdleTimeout:    60 * time.Second,
		},
	}
	if opts.h3DialGuard != nil {
		transport.Dial = guardedH3Dial(opts.h3DialGuard)
	}
	if opts.h3DialGuardContext != nil {
		transport.Dial = guardedH3DialContext(opts.h3DialGuardContext)
	}

	return transport, nil
}

func guardedH3Dial(guard func(net.IP) error) func(context.Context, string, *tls.Config, *quic.Config) (*quic.Conn, error) {
	return guardedH3DialContext(func(_ context.Context, ip net.IP) error {
		return guard(ip)
	})
}

func guardedH3DialContext(guard func(context.Context, net.IP) error) func(context.Context, string, *tls.Config, *quic.Config) (*quic.Conn, error) {
	return func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
		udpAddr, err := resolveH3DialUDPAddr(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("resolve h3 dial addr %s: %w", addr, err)
		}
		if guardErr := guard(ctx, udpAddr.IP); guardErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrH3EgressDenied, guardErr)
		}
		return quic.DialAddrEarly(ctx, udpAddr.String(), tlsCfg, cfg)
	}
}

func resolveH3DialUDPAddr(ctx context.Context, addr string) (*net.UDPAddr, error) {
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := net.DefaultResolver.LookupPort(ctx, "udp", portString)
	if err != nil {
		return nil, err
	}
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ipAddrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	ipAddr := selectH3DialIPAddr(ipAddrs, addr)
	return &net.UDPAddr{IP: ipAddr.IP, Port: port, Zone: ipAddr.Zone}, nil
}

func selectH3DialIPAddr(ipAddrs []net.IPAddr, addr string) net.IPAddr {
	wantIPv6 := strings.Contains(addr, "[")
	for _, ipAddr := range ipAddrs {
		if (ipAddr.IP.To4() == nil) == wantIPv6 {
			return ipAddr
		}
	}
	return ipAddrs[0]
}

func resolveH3CAReloadInterval(opts clientOptions) time.Duration {
	if opts.h3CAReloadInterval > 0 {
		return opts.h3CAReloadInterval
	}
	return parseDurationEnv(os.Getenv(envH3CAReloadInterval))
}

func parseDurationEnv(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0
	}
	return d
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

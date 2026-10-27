package transport

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

// shared-go pkg/h3의 client QUIC 설정과 같은 값을 쓴다. 두 경로가 같은 overlay를 지난다.
const (
	// Overlay(Tailscale) MTU에서 fragmentation 없이 통과하는 보수값.
	h3InitialPacketSize = 1200

	h3KeepAlivePeriod = 10 * time.Second
	h3MaxIdleTimeout  = 60 * time.Second

	// 죽은 피어에 묶이지 않도록 quic-go 기본(5s)보다 약간 관대한 핸드셰이크 상한.
	h3HandshakeIdleTimeout = 10 * time.Second
)

// H3 신뢰 루트는 WithH3CACertFile로 명시한 CA 하나다. 시스템 루트로 넘어가는 경로는 두지 않는다.
var (
	ErrEmptyH3CACertFile   = errors.New("iris: H3 CA cert file is empty; refusing to fall back to system roots")
	ErrMissingH3CACertFile = errors.New("iris: H3 CA cert file is required (use WithH3CACertFile)")
)

func newHTTP3Transport(opts clientOptions) (*http3.Transport, error) {
	if opts.h3CACertFile == "" {
		return nil, ErrMissingH3CACertFile
	}

	// #nosec G304 -- CA 인증서 경로는 운영자 소유 설정이며 사용자 입력이 아니다.
	pemBytes, err := os.ReadFile(opts.h3CACertFile)
	if err != nil {
		return nil, fmt.Errorf("read H3 CA cert file: %w", err)
	}

	return newHTTP3TransportFromCA(opts, pemBytes)
}

func newHTTP3TransportFromCA(opts clientOptions, pemBytes []byte) (*http3.Transport, error) {
	if len(pemBytes) == 0 {
		return nil, ErrEmptyH3CACertFile
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("iris: parse H3 CA cert file: no PEM certificate")
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool}

	if opts.h3ServerName != "" {
		tlsCfg.ServerName = opts.h3ServerName
	}

	transport := &http3.Transport{
		TLSClientConfig: tlsCfg,
		QUICConfig: &quic.Config{
			InitialPacketSize:    h3InitialPacketSize,
			HandshakeIdleTimeout: h3HandshakeIdleTimeout,
			KeepAlivePeriod:      h3KeepAlivePeriod,
			MaxIdleTimeout:       h3MaxIdleTimeout,
		},
	}

	if opts.h3DialGuard != nil {
		transport.Dial = guardedH3DialContext(opts.h3DialGuard)
	}

	return transport, nil
}

func guardedH3DialContext(guard func(context.Context, net.IP) error) func(context.Context, string, *tls.Config, *quic.Config) (*quic.Conn, error) {
	return func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
		// 아래 실패는 모두 QUIC dial 전이라 새 연결로 요청이 나가지 않았음이 확정된다. ErrNotSent로
		// 표시해 소비자가 결과 불명(admission 가능성)으로 오분류하지 않게 한다.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: check h3 dial context before resolve: %w", ErrNotSent, ctxErr)
		}

		udpAddr, err := resolveH3DialUDPAddr(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve h3 dial addr %s: %w", ErrNotSent, addr, err)
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%w: check h3 dial context after resolve: %w", ErrNotSent, ctxErr)
		}

		if guardErr := guard(ctx, udpAddr.IP); guardErr != nil {
			return nil, fmt.Errorf("%w: %w: %w", ErrNotSent, ErrH3EgressDenied, guardErr)
		}

		return quic.DialAddrEarly(ctx, udpAddr.String(), tlsCfg, cfg)
	}
}

func resolveH3DialUDPAddr(ctx context.Context, addr string) (*net.UDPAddr, error) {
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split host port: %w", err)
	}

	port, err := net.DefaultResolver.LookupPort(ctx, "udp", portString)
	if err != nil {
		return nil, fmt.Errorf("lookup port: %w", err)
	}

	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("lookup IP address: %w", err)
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

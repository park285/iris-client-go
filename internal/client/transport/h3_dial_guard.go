package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"
)

const (
	defaultH3DialGuardTTL            = time.Minute
	defaultH3DialGuardResolveTimeout = 5 * time.Second
)

type H3DialGuardOption func(*h3DialGuardOptions)

type h3DialGuardOptions struct {
	ttl            time.Duration
	resolveTimeout time.Duration
	lenientInit    bool
	logger         *slog.Logger
}

type h3DialGuardDependencies struct {
	lookupIP func(context.Context, string) ([]net.IP, error)
	now      func() time.Time
}

type h3DialGuard struct {
	mu             sync.Mutex
	host           string
	allowed        map[string]struct{}
	expiresAt      time.Time
	refreshDone    chan struct{}
	ttl            time.Duration
	resolveTimeout time.Duration
	logger         *slog.Logger
	lookupIP       func(context.Context, string) ([]net.IP, error)
	now            func() time.Time
}

func WithH3DialGuardTTL(ttl time.Duration) H3DialGuardOption {
	return func(opts *h3DialGuardOptions) {
		if ttl > 0 {
			opts.ttl = ttl
		}
	}
}

func WithH3DialGuardResolveTimeout(timeout time.Duration) H3DialGuardOption {
	return func(opts *h3DialGuardOptions) {
		if timeout > 0 {
			opts.resolveTimeout = timeout
		}
	}
}

func WithH3DialGuardLenientInit() H3DialGuardOption {
	return func(opts *h3DialGuardOptions) {
		opts.lenientInit = true
	}
}

func WithH3DialGuardLogger(logger *slog.Logger) H3DialGuardOption {
	return func(opts *h3DialGuardOptions) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

func NewH3DialGuardForBaseURL(
	ctx context.Context,
	baseURL string,
	opts ...H3DialGuardOption,
) (func(context.Context, net.IP) error, error) {
	return newH3DialGuardForBaseURL(ctx, baseURL, defaultH3DialGuardDependencies(), opts...)
}

func WithH3DialGuardForBaseURL(
	ctx context.Context,
	baseURL string,
	opts ...H3DialGuardOption,
) (ClientOption, error) {
	return withH3DialGuardForBaseURL(ctx, baseURL, opts...)
}

func withH3DialGuardForBaseURL(
	ctx context.Context,
	baseURL string,
	opts ...H3DialGuardOption,
) (ClientOption, error) {
	guard, err := newH3DialGuardForBaseURL(ctx, baseURL, defaultH3DialGuardDependencies(), opts...)
	if err != nil {
		return nil, err
	}
	return WithH3DialGuardContext(guard), nil
}

func newH3DialGuardForBaseURL(
	ctx context.Context,
	baseURL string,
	deps h3DialGuardDependencies,
	opts ...H3DialGuardOption,
) (func(context.Context, net.IP) error, error) {
	host, err := parseH3DialGuardHost(baseURL)
	if err != nil {
		return nil, err
	}
	cfg := applyH3DialGuardOptions(opts)
	guard := &h3DialGuard{
		host:           host,
		ttl:            cfg.ttl,
		resolveTimeout: cfg.resolveTimeout,
		logger:         cfg.logger,
		lookupIP:       deps.lookupIP,
		now:            deps.now,
	}
	if literalIP := net.ParseIP(host); literalIP != nil {
		guard.allowed = ipAllowset([]net.IP{literalIP})
		return guard.allow, nil
	}
	if err := guard.initialize(ctx, cfg.lenientInit); err != nil {
		return nil, err
	}
	return guard.allow, nil
}

func defaultH3DialGuardDependencies() h3DialGuardDependencies {
	return h3DialGuardDependencies{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		},
		now: time.Now,
	}
}

func applyH3DialGuardOptions(opts []H3DialGuardOption) h3DialGuardOptions {
	out := h3DialGuardOptions{
		ttl:            defaultH3DialGuardTTL,
		resolveTimeout: defaultH3DialGuardResolveTimeout,
		logger:         slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func parseH3DialGuardHost(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("iris: parse H3 dial guard base URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("iris: H3 dial guard base URL must be absolute and include a host")
	}
	return parsed.Hostname(), nil
}

func (g *h3DialGuard) initialize(ctx context.Context, lenient bool) error {
	allowed, err := g.resolve(ctx)
	if err == nil {
		g.allowed = allowed
		g.expiresAt = g.now().Add(g.ttl)
		return nil
	}
	if !lenient {
		return fmt.Errorf("iris: resolve H3 dial guard host %s: %w", g.host, err)
	}
	g.allowed = make(map[string]struct{})
	g.expiresAt = g.now().Add(g.ttl)
	g.logInitialResolveFailure(err)
	return nil
}

func (g *h3DialGuard) allow(ctx context.Context, ip net.IP) error {
	key := canonicalIP(ip)

	// allowset은 refresh 판정 뒤에 읽는다. 먼저 읽으면 그 사이 끝난 refresh의 결과를 놓친
	// 채 "만료 아님"으로 넘어가 멀쩡한 IP를 거부할 수 있다.
	refreshDone := g.beginRefresh(ctx)

	// 허용된 dial은 기존대로 stale allowset으로 즉시 통과하고 refresh는 뒤에서 끝낸다.
	// 만료된 allowset 때문에 거부된 dial만 그 refresh 결과를 기다렸다 한 번 더 판정한다.
	// 그러지 않으면 IP가 바뀐 TTL 경계마다 요청 한 건이 거부로 희생된다.
	if g.permits(key) {
		return nil
	}
	if refreshDone == nil {
		return h3EgressDeniedError(g.host, ip)
	}

	select {
	case <-refreshDone:
		if g.permits(key) {
			return nil
		}
	case <-ctx.Done():
	}

	return h3EgressDeniedError(g.host, ip)
}

func h3EgressDeniedError(host string, ip net.IP) error {
	return fmt.Errorf("iris: H3 egress denied for host %s and IP %v", host, ip)
}

func (g *h3DialGuard) permits(key string) bool {
	if key == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	_, allowed := g.allowed[key]
	return allowed
}

// refresh는 항상 detached goroutine에서 돌리고 동기 경로는 완료만 기다린다. panic 복구와
// context 분리 규칙이 두 경로에서 갈라지지 않게 하기 위해서다. 반환값 nil은 allowset이
// 아직 유효해 refresh가 필요 없다는 뜻이다.
func (g *h3DialGuard) beginRefresh(ctx context.Context) <-chan struct{} {
	g.mu.Lock()
	if g.expiresAt.IsZero() || g.now().Before(g.expiresAt) {
		g.mu.Unlock()
		return nil
	}
	if done := g.refreshDone; done != nil {
		g.mu.Unlock()
		return done
	}
	done := make(chan struct{})
	g.refreshDone = done
	g.mu.Unlock()

	safeGo(g.logger, "H3 dial guard refresh panicked", func() {
		g.refresh(context.WithoutCancel(ctx))
	})
	return done
}

func (g *h3DialGuard) refresh(ctx context.Context) {
	defer func() {
		g.mu.Lock()
		done := g.refreshDone
		g.expiresAt = g.now().Add(g.ttl)
		g.refreshDone = nil
		g.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	allowed, err := g.resolve(ctx)
	g.mu.Lock()
	if err == nil {
		g.allowed = allowed
	}
	g.mu.Unlock()
	if err != nil {
		g.logResolveFailure(err)
	}
}

func (g *h3DialGuard) resolve(ctx context.Context) (map[string]struct{}, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, g.resolveTimeout)
	defer cancel()
	ips, err := g.lookupIP(resolveCtx, g.host)
	if err != nil {
		return nil, err
	}
	allowed := ipAllowset(ips)
	if len(allowed) == 0 {
		return nil, fmt.Errorf("DNS returned no IP addresses")
	}
	return allowed, nil
}

func (g *h3DialGuard) logInitialResolveFailure(err error) {
	g.logger.Warn(
		"H3 dial guard initial DNS resolve failed; starting with deny-all allowset",
		slog.String("host", g.host),
		slog.Any("err", err),
	)
}

func (g *h3DialGuard) logResolveFailure(err error) {
	g.logger.Warn(
		"H3 dial guard DNS refresh failed; keeping stale allowset",
		slog.String("host", g.host),
		slog.Any("err", err),
	)
}

func ipAllowset(ips []net.IP) map[string]struct{} {
	allowed := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if key := canonicalIP(ip); key != "" {
			allowed[key] = struct{}{}
		}
	}
	return allowed
}

func canonicalIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return string(v4)
	}
	if v6 := ip.To16(); v6 != nil {
		return string(v6)
	}
	return ""
}

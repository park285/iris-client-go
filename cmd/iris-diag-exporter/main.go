package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/park285/iris-client-go/iris"
)

const (
	pollTimeout           = 8 * time.Second
	defaultListenAddr     = "127.0.0.1:9105"
	defaultIrisBaseURL    = "https://localhost:3001"
	defaultIrisCACertFile = "/run/iris/certs/iris-ca.pem"
)

// metricKeyAllowlist는 diagnostics JSON에서 metric으로 방출을 허용하는 flatten key 집합이다.
// 임의 key를 metric으로 펼치면 cardinality 폭증과 이름 충돌이 발생하므로, 알려진 운영 지표만 허용한다.
var metricKeyAllowlist = map[string]struct{}{
	"iris_workers_reply_running":                         {},
	"iris_workers_reply_success_count":                   {},
	"iris_http_metrics_h3_active_connections":            {},
	"iris_http_metrics_h3_active_streams":                {},
	"iris_workers_webhook_backlog_pending":               {},
	"iris_workers_webhook_backlog_retry":                 {},
	"iris_workers_webhook_backlog_dead":                  {},
	"iris_workers_webhook_backlog_oldest_pending_age_ms": {},
	"iris_workers_webhook_dead_deliveries":               {},
	"iris_workers_webhook_retry_deliveries":              {},
	"iris_workers_webhook_breaker_open_total":            {},
	"iris_h3_cert_remaining_days":                        {},
	"iris_workers_webhook_webhook_lane_idle_polls":       {},
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	listen := envOr("IRIS_DIAG_EXPORTER_LISTEN", defaultListenAddr)
	token := strings.TrimSpace(os.Getenv("IRIS_DIAG_EXPORTER_TOKEN"))
	if err := validateExporterExposure(listen, token); err != nil {
		logger.Error("iris-diag-exporter refusing to start", "err", err)
		os.Exit(1)
	}

	baseURL := envOr("IRIS_BASE_URL", defaultIrisBaseURL)
	caFile := envOr("IRIS_DIAG_EXPORTER_CA_FILE", defaultIrisCACertFile)

	c, err := iris.NewClient(
		iris.WithBaseURL(baseURL),
		iris.WithTransport("h3"),
		iris.WithH3CACertFile(caFile),
		iris.WithTimeout(pollTimeout),
		iris.WithLogger(logger),
	)
	if err != nil {
		logger.Error("iris client init failed", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/metrics", metricsHandler(logger, token, func(ctx context.Context) ([]byte, error) {
		return c.GetRuntimeDiagnostics(ctx)
	}))

	srv := &http.Server{
		Addr:              listen,
		Handler:           RecoverHTTP(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("iris-diag-exporter listening", "addr", listen, "iris", baseURL, "auth", token != "")
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("listener failed", "err", err)
		os.Exit(1)
	}
}

func metricsHandler(logger *slog.Logger, token string, fetch func(context.Context) ([]byte, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeMetrics(r, token) {
			logger.Warn("diag exporter auth failed", "remote", r.RemoteAddr)
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()
		raw, err := fetch(ctx)
		dur := time.Since(start).Seconds()
		if err != nil {
			logger.Warn("diagnostics poll failed", "err", err)
			writeScrapeStatus(w, false, dur)
			return
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			logger.Warn("diagnostics decode failed", "err", err)
			writeScrapeStatus(w, false, dur)
			return
		}

		writeScrapeStatus(w, true, dur)
		if m, ok := doc.(map[string]any); ok {
			if state, ok := m["state"].(string); ok && state != "" {
				_, _ = fmt.Fprintf(w, "iris_runtime_state_info{state=%q} 1\n", state)
			}
		}
		series := map[string]float64{}
		collisions := flatten(series, "iris", doc)
		_, _ = fmt.Fprintf(w, "iris_flatten_name_collisions %d\n", collisions)
		emitted, dropped := allowlistedSeries(series)
		_, _ = fmt.Fprintf(w, "iris_flatten_dropped_keys %d\n", dropped)
		names := make([]string, 0, len(emitted))
		for name := range emitted {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintf(w, "%s %g\n", name, emitted[name])
		}
	}
}

func writeScrapeStatus(w http.ResponseWriter, up bool, duration float64) {
	value := 0
	if up {
		value = 1
	}
	_, _ = fmt.Fprintf(w, "iris_up %d\niris_scrape_duration_seconds %g\n", value, duration)
}

func RecoverHTTP(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := newBufferedHTTPResponse()
		defer finishHTTPResponse(r.Context(), logger, w, response)

		next.ServeHTTP(response, r)
	})
}

type bufferedHTTPResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedHTTPResponse() *bufferedHTTPResponse {
	return &bufferedHTTPResponse{header: make(http.Header)}
}

func (w *bufferedHTTPResponse) Header() http.Header {
	return w.header
}

func (w *bufferedHTTPResponse) WriteHeader(status int) {
	if status < 100 || status > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %d", status))
	}
	if w.status == 0 {
		w.status = status
	}
}

func (w *bufferedHTTPResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *bufferedHTTPResponse) commit(dst http.ResponseWriter) {
	for name, values := range w.header {
		dst.Header()[name] = append([]string(nil), values...)
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = dst.Write(w.body.Bytes())
}

func finishHTTPResponse(ctx context.Context, logger *slog.Logger, dst http.ResponseWriter, response *bufferedHTTPResponse) {
	recovered := recover()
	if recovered == nil {
		response.commit(dst)
		return
	}
	if recoveredErr, ok := recovered.(error); ok && errors.Is(recoveredErr, http.ErrAbortHandler) {
		panic(recovered)
	}
	logger.ErrorContext(
		ctx,
		"diag_exporter_http_panic_recovered",
		slog.String("panic_type", fmt.Sprintf("%T", recovered)),
		slog.String("stack", string(debug.Stack())),
	)
	http.Error(dst, "internal server error", http.StatusInternalServerError)
}

// validateExporterExposure는 non-loopback bind를 token 없이 노출하려는 silent unauth 구성을 거부한다.
func validateExporterExposure(listen, token string) error {
	if token != "" {
		return nil
	}
	if isLoopbackListen(listen) {
		return nil
	}
	return fmt.Errorf("IRIS_DIAG_EXPORTER_LISTEN=%q exposes /metrics off-loopback without IRIS_DIAG_EXPORTER_TOKEN; set a token or bind to 127.0.0.1", listen)
}

func isLoopbackListen(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		host = listen
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func authorizeMetrics(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimSpace(header[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

// allowlistedSeries는 allowlist에 있는 metric만 남기고, drop된 key 수를 반환한다.
func allowlistedSeries(series map[string]float64) (map[string]float64, int) {
	emitted := make(map[string]float64, len(series))
	dropped := 0
	for name, value := range series {
		if _, ok := metricKeyAllowlist[name]; ok {
			emitted[name] = value
			continue
		}
		dropped++
	}
	return emitted, dropped
}

func flatten(out map[string]float64, prefix string, v any) int {
	collisions := 0
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			collisions += flatten(out, prefix+"_"+snake(k), vv)
		}
	case float64:
		if _, dup := out[prefix]; dup {
			collisions++
		}
		out[prefix] = t
	case bool:
		if _, dup := out[prefix]; dup {
			collisions++
		}
		if t {
			out[prefix] = 1
		} else {
			out[prefix] = 0
		}
	}
	return collisions
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

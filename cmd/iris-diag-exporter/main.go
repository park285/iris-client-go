package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/park285/iris-client-go/iris"
)

const (
	pollTimeout       = 8 * time.Second
	defaultListenAddr = "127.0.0.1:9105"
)

// metricKeyAllowlist는 diagnostics JSON에서 metric으로 방출을 허용하는 flatten key 집합이다.
// 임의 key를 metric으로 펼치면 cardinality 폭증과 이름 충돌이 발생하므로, 알려진 운영 지표만 허용한다.
var metricKeyAllowlist = map[string]struct{}{
	"iris_workers_reply_running":                       {},
	"iris_workers_reply_success_count":                 {},
	"iris_workers_reply_backlog_pending":               {},
	"iris_workers_reply_backlog_oldest_pending_age_ms": {},
	"iris_http_metrics_h3_active_connections":          {},
	"iris_http_metrics_h3_active_streams":              {},
	"iris_webhook_lane_idle_polls":                     {},
	"iris_webhook_lane_backlog":                        {},
	"iris_uptime_seconds":                              {},
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	listen := envOr("IRIS_DIAG_EXPORTER_LISTEN", defaultListenAddr)
	token := strings.TrimSpace(os.Getenv("IRIS_DIAG_EXPORTER_TOKEN"))
	if err := validateExporterExposure(listen, token); err != nil {
		logger.Error("iris-diag-exporter refusing to start", "err", err)
		os.Exit(1)
	}

	baseURL := envOr("IRIS_BASE_URL", "https://100.100.1.5:3001")
	caFile := envOr("IRIS_DIAG_EXPORTER_CA_FILE", "/run/iris/certs/iris-ca.pem")

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
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
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
		raw, err := c.GetRuntimeDiagnostics(ctx)
		dur := time.Since(start).Seconds()
		if err != nil {
			logger.Warn("diagnostics poll failed", "err", err)
			_, _ = fmt.Fprintf(w, "iris_up 0\niris_scrape_duration_seconds %g\n", dur)
			return
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			logger.Warn("diagnostics decode failed", "err", err)
			_, _ = fmt.Fprintf(w, "iris_up 0\niris_scrape_duration_seconds %g\n", dur)
			return
		}

		_, _ = fmt.Fprintf(w, "iris_up 1\niris_scrape_duration_seconds %g\n", dur)
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
	})

	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("iris-diag-exporter listening", "addr", listen, "iris", baseURL, "auth", token != "")
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("listener failed", "err", err)
		os.Exit(1)
	}
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
	host := listen
	if h, _, err := splitHostPort(listen); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	switch host {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return strings.HasPrefix(host, "127.")
}

func splitHostPort(addr string) (string, string, error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("no port in %q", addr)
	}
	host := strings.Trim(addr[:idx], "[]")
	return host, addr[idx+1:], nil
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

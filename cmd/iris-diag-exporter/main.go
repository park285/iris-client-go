package main

import (
	"context"
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

const pollTimeout = 8 * time.Second

func main() {
	listen := envOr("IRIS_DIAG_EXPORTER_LISTEN", "100.100.1.5:9105")
	baseURL := envOr("IRIS_BASE_URL", "https://100.100.1.5:3001")
	caFile := envOr("IRIS_DIAG_EXPORTER_CA_FILE", "/run/iris/certs/iris-ca.pem")

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

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
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), pollTimeout)
		defer cancel()
		raw, err := c.GetRuntimeDiagnostics(ctx)
		dur := time.Since(start).Seconds()
		if err != nil {
			logger.Warn("diagnostics poll failed", "err", err)
			fmt.Fprintf(w, "iris_up 0\niris_scrape_duration_seconds %g\n", dur)
			return
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			logger.Warn("diagnostics decode failed", "err", err)
			fmt.Fprintf(w, "iris_up 0\niris_scrape_duration_seconds %g\n", dur)
			return
		}

		fmt.Fprintf(w, "iris_up 1\niris_scrape_duration_seconds %g\n", dur)
		if m, ok := doc.(map[string]any); ok {
			if state, ok := m["state"].(string); ok && state != "" {
				fmt.Fprintf(w, "iris_runtime_state_info{state=%q} 1\n", state)
			}
		}
		series := map[string]float64{}
		collisions := flatten(series, "iris", doc)
		fmt.Fprintf(w, "iris_flatten_name_collisions %d\n", collisions)
		names := make([]string, 0, len(series))
		for name := range series {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(w, "%s %g\n", name, series[name])
		}
	})

	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("iris-diag-exporter listening", "addr", listen, "iris", baseURL)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("listener failed", "err", err)
		os.Exit(1)
	}
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverHTTPContainsHandlerPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := RecoverHTTP(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if body := recorder.Body.String(); body != "internal server error\n" {
		t.Fatalf("body = %q, want generic internal server error", body)
	}
}

func TestRecoverHTTPDiscardsPartialResponseOnPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := RecoverHTTP(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/private")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial sensitive response"))
		panic("boom")
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if body := recorder.Body.String(); body != "internal server error\n" {
		t.Fatalf("body = %q, want only generic internal server error", body)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want generic error content type", got)
	}
}

func TestRecoverHTTPCommitsSuccessfulResponse(t *testing.T) {
	t.Parallel()

	handler := RecoverHTTP(nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("X-Test", "one")
		w.Header().Add("X-Test", "two")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if body := recorder.Body.String(); body != "created" {
		t.Fatalf("body = %q, want created", body)
	}
	if got := recorder.Header().Values("X-Test"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("X-Test = %v, want [one two]", got)
	}
}

func TestRecoverHTTPPreservesAbortHandlerPanic(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := RecoverHTTP(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		recovered := recover()
		recoveredErr, ok := recovered.(error)
		if !ok || !errors.Is(recoveredErr, http.ErrAbortHandler) {
			t.Fatalf("recovered = %v, want http.ErrAbortHandler", recovered)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", nil))
	t.Fatal("RecoverHTTP swallowed http.ErrAbortHandler")
}

func TestSnake(t *testing.T) {
	cases := map[string]string{
		"oldestPendingAgeMs":   "oldest_pending_age_ms",
		"backlog":              "backlog",
		"h3ActiveConnections":  "h3_active_connections",
		"successCount":         "success_count",
		"webhookLaneIdlePolls": "webhook_lane_idle_polls",
	}
	for in, want := range cases {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlatten(t *testing.T) {
	raw := []byte(`{
		"state": "running",
		"workers": {
			"reply": {
				"running": true,
				"successCount": 42,
				"backlog": {"pending": 3, "oldestPendingAgeMs": 1500}
			}
		},
		"httpMetrics": {"h3ActiveConnections": 2},
		"detail": null,
		"tags": ["a", "b"]
	}`)
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	if collisions := flatten(out, "iris", doc); collisions != 0 {
		t.Errorf("collisions = %d, want 0", collisions)
	}

	want := map[string]float64{
		"iris_workers_reply_running":                       1,
		"iris_workers_reply_success_count":                 42,
		"iris_workers_reply_backlog_pending":               3,
		"iris_workers_reply_backlog_oldest_pending_age_ms": 1500,
		"iris_http_metrics_h3_active_connections":          2,
	}
	for name, val := range want {
		if got, ok := out[name]; !ok || got != val {
			t.Errorf("out[%q] = %v (present=%v), want %v", name, got, ok, val)
		}
	}
	if _, ok := out["iris_state"]; ok {
		t.Error("string field must not be flattened")
	}
	if len(out) != len(want) {
		t.Errorf("unexpected extra series: got %d, want %d — %v", len(out), len(want), out)
	}
}

func TestDiagExporterAllowlistTracksRuntimeDiagnostics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		backlog         string
		wantAbsent      []string
		allowlistAbsent []string
		wantZero        []string
	}{
		{
			name:    "backlog pending",
			backlog: `"backlog":{"pending":3,"retry":2,"dead":1,"oldestPendingAgeMs":1500}`,
			wantAbsent: []string{
				"iris_workers_reply_backlog_pending",
				"iris_workers_reply_backlog_oldest_pending_age_ms",
				"iris_webhook_lane_idle_polls",
				"iris_webhook_lane_backlog",
				"iris_uptime_seconds",
			},
		},
		{
			name:    "backlog empty",
			backlog: `"backlog":{"pending":0,"retry":0,"dead":0}`,
			wantAbsent: []string{
				"iris_workers_reply_backlog_pending",
				"iris_workers_reply_backlog_oldest_pending_age_ms",
				"iris_webhook_lane_idle_polls",
				"iris_webhook_lane_backlog",
				"iris_uptime_seconds",
			},
			allowlistAbsent: []string{
				"iris_workers_webhook_backlog_oldest_pending_age_ms",
			},
			wantZero: []string{
				"iris_workers_webhook_backlog_pending",
				"iris_workers_webhook_backlog_retry",
				"iris_workers_webhook_backlog_dead",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{
				"workers": {
					"reply": {"running": true, "successCount": 42, "backlog": {"pending": 9, "oldestPendingAgeMs": 2000}},
				"webhook": {"running": true, "webhookLaneIdlePolls": 7, "deadDeliveries": 11, "retryDeliveries": 12, "breakerOpenTotal": 13, ` + tc.backlog + `}
				},
				"httpMetrics": {"h3ActiveConnections": 2, "h3ActiveStreams": 4},
				"h3Cert": {"remainingDays": 30},
				"webhookLaneIdlePolls": 8,
				"webhookLaneBacklog": 9,
				"uptimeSeconds": 999
			}`)
			var doc any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			series := map[string]float64{}
			flatten(series, "iris", doc)
			emitted, _ := allowlistedSeries(series)

			for name := range metricKeyAllowlist {
				if contains(tc.allowlistAbsent, name) {
					if _, ok := emitted[name]; ok {
						t.Errorf("allowlisted key %q was emitted", name)
					}
					continue
				}
				if got, ok := emitted[name]; !ok {
					t.Errorf("allowlisted key %q is absent from emitted series", name)
				} else if tc.name == "backlog pending" && got == 0 {
					t.Errorf("emitted[%q] = 0, want non-zero fixture value", name)
				}
			}
			for _, name := range tc.wantZero {
				if got, ok := emitted[name]; !ok {
					t.Errorf("zero-valued series %q is absent", name)
				} else if got != 0 {
					t.Errorf("emitted[%q] = %v, want 0", name, got)
				}
			}
			for _, name := range tc.wantAbsent {
				if _, ok := emitted[name]; ok {
					t.Errorf("dead key %q was emitted", name)
				}
			}
		})
	}
}

func TestDiagExporterAlwaysEmitsIrisUp(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		name       string
		fetch      func(context.Context) ([]byte, error)
		want       string
		wantSeries bool
	}{
		{
			name:  "poll failure",
			fetch: func(context.Context) ([]byte, error) { return nil, errors.New("dial timeout") },
			want:  "iris_up 0\n",
		},
		{
			name:  "decode failure",
			fetch: func(context.Context) ([]byte, error) { return []byte("not-json"), nil },
			want:  "iris_up 0\n",
		},
		{
			name: "success",
			fetch: func(context.Context) ([]byte, error) {
				return []byte(`{"workers":{"webhook":{"backlog":{"dead":3}}}}`), nil
			},
			want:       "iris_up 1\n",
			wantSeries: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			metricsHandler(logger, "", tc.fetch).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			body := recorder.Body.String()
			if !strings.Contains(body, tc.want) {
				t.Fatalf("output = %q, want contains %q", body, tc.want)
			}
			if got := strings.Contains(body, "iris_workers_webhook_backlog_dead 3"); got != tc.wantSeries {
				t.Fatalf("series emitted = %v, want %v (body %q)", got, tc.wantSeries, body)
			}
		})
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestFlattenNameCollision(t *testing.T) {
	var doc any
	if err := json.Unmarshal([]byte(`{"fooBar": 1, "foo_bar": 2}`), &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	if collisions := flatten(out, "iris", doc); collisions != 1 {
		t.Errorf("collisions = %d, want 1", collisions)
	}
	if len(out) != 1 {
		t.Errorf("series count = %d, want 1", len(out))
	}
}

func TestIC01DiagExporterBindsLoopbackByDefault_31b1c654(t *testing.T) {
	if defaultListenAddr != "127.0.0.1:9105" {
		t.Fatalf("defaultListenAddr = %q, want 127.0.0.1:9105", defaultListenAddr)
	}
	if got := envOr("IRIS_DIAG_EXPORTER_LISTEN", defaultListenAddr); !isLoopbackListen(got) {
		t.Fatalf("default listen %q is not loopback", got)
	}
}

func TestIC01DiagExporterNonLoopbackWithoutTokenFailsStartup_31b1c654(t *testing.T) {
	cases := []struct {
		name    string
		listen  string
		token   string
		wantErr bool
	}{
		{"loopback no token", "127.0.0.1:9105", "", false},
		{"localhost no token", "localhost:9105", "", false},
		{"ipv6 loopback no token", "[::1]:9105", "", false},
		{"tailnet no token", "100.100.1.5:9105", "", true},
		{"wildcard no token", "0.0.0.0:9105", "", true},
		{"tailnet with token", "100.100.1.5:9105", "secret", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExporterExposure(tc.listen, tc.token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateExporterExposure(%q, token=%q) err = %v, wantErr = %v", tc.listen, tc.token, err, tc.wantErr)
			}
		})
	}
}

func TestLoopbackListen_EmptyHost_74c2d97f(t *testing.T) {
	cases := []struct {
		name       string
		listen     string
		token      string
		wantReject bool
	}{
		{"empty host no token", ":9105", "", true},
		{"empty host with token", ":9105", "tok", false},
		{"wildcard v4 no token", "0.0.0.0:9105", "", true},
		{"wildcard v6 no token", "[::]:9105", "", true},
		{"loopback v4 no token", "127.0.0.1:9105", "", false},
		{"loopback v6 no token", "[::1]:9105", "", false},
		{"localhost no token", "localhost:9105", "", false},
		{"tailnet no token", "100.100.1.5:9105", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExporterExposure(tc.listen, tc.token)
			if tc.wantReject != (err != nil) {
				t.Fatalf("validateExporterExposure(%q, token=%q) err = %v, wantReject = %v", tc.listen, tc.token, err, tc.wantReject)
			}
		})
	}
}

func TestIC01DiagExporterMetricsRequiresToken_31b1c654(t *testing.T) {
	const token = "super-secret-token"

	authorized := func(authHeader string) bool {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		return authorizeMetrics(req, token)
	}

	if !authorized("Bearer " + token) {
		t.Fatal("matching bearer token must be authorized")
	}
	if authorized("Bearer wrong-token") {
		t.Fatal("mismatched bearer token must be rejected")
	}
	if authorized("") {
		t.Fatal("missing Authorization header must be rejected when token is set")
	}
	if authorized(token) {
		t.Fatal("non-bearer scheme must be rejected")
	}

	reqNoToken := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if !authorizeMetrics(reqNoToken, "") {
		t.Fatal("loopback-only deployment (no token) must allow unauthenticated scrape")
	}
}

func TestIC01DiagExporterFlattenUsesAllowlist_31b1c654(t *testing.T) {
	raw := []byte(`{
		"workers": {"reply": {"successCount": 7}},
		"attackerControlled": {"deeplyNested": {"key": 999}},
		"randomCardinalityBomb": 123
	}`)
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	series := map[string]float64{}
	flatten(series, "iris", doc)

	emitted, dropped := allowlistedSeries(series)
	if _, ok := emitted["iris_workers_reply_success_count"]; !ok {
		t.Fatal("allowlisted metric must be emitted")
	}
	if _, ok := emitted["iris_attacker_controlled_deeply_nested_key"]; ok {
		t.Fatal("non-allowlisted key must not be emitted as a metric")
	}
	if _, ok := emitted["iris_random_cardinality_bomb"]; ok {
		t.Fatal("non-allowlisted scalar must not be emitted as a metric")
	}
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2", dropped)
	}
}

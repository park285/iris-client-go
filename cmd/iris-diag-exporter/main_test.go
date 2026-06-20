package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

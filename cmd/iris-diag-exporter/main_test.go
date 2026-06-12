package main

import (
	"encoding/json"
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

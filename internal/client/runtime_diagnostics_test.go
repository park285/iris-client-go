package client

import (
	"errors"
	"testing"
)

func TestDecodeIrisBotWebhookPipelineDiagnostics(t *testing.T) {
	raw := []byte(`{
		"workers":{"webhook":{"webhookPipeline":{
			"profileEnabled":true,
			"profileVersion":1,
			"profileId":"main",
			"profileHash":"abc123",
			"workerProfile":{
				"version":1,
				"profile_id":"main",
				"delivery":{
					"lane_workers":32,
					"lane_queue_capacity":128,
					"max_global_in_flight":32,
					"max_per_endpoint_in_flight":8,
					"max_drain_per_tick":128,
					"max_attempts":6,
					"request_timeout_ms":125000,
					"lane_idle_timeout_ms":750,
					"breaker_failure_threshold":5,
					"breaker_cooldown_ms":30000
				},
				"receive":{
					"workers":16,
					"queue_size":1000,
					"enqueue_timeout_ms":50,
					"handler_timeout_ms":120000,
					"max_body_bytes":65536,
					"dedup_ttl_ms":60000,
					"dedup_timeout_ms":200
				},
				"bot_pool":{"workers":10,"queue_size":100},
				"validation":{
					"min_queue_per_endpoint_multiplier":4,
					"require_receive_capacity_for_endpoint_burst":true
				}
			},
			"delivery":{
				"workersConfigured":32,
				"queueCapacity":128,
				"maxGlobalInFlight":32,
				"maxPerEndpointInFlight":8,
				"maxDrainPerTick":128,
				"maxAttempts":6,
				"requestTimeoutMs":125000,
				"laneIdleTimeoutMs":750,
				"breakerFailureThreshold":5,
				"breakerCooldownMs":30000
			},
			"receiveExpected":{
				"workersExpected":16,
				"queueSizeExpected":1000,
				"enqueueTimeoutMs":50,
				"handlerTimeoutMs":120000,
				"maxBodyBytes":65536,
				"dedupTtlMs":60000,
				"dedupTimeoutMs":200
			},
			"botPoolExpected":{"workersExpected":10,"queueSizeExpected":100}
		}}}
	}`)

	pipeline, err := DecodeIrisBotWebhookPipelineDiagnostics(raw)
	if err != nil {
		t.Fatalf("DecodeIrisBotWebhookPipelineDiagnostics() error = %v", err)
	}

	if pipeline.ProfileID != "main" || pipeline.ProfileHash != "abc123" {
		t.Fatalf("pipeline identity = %q/%q, want main/abc123", pipeline.ProfileID, pipeline.ProfileHash)
	}
	if pipeline.WorkerProfile == nil {
		t.Fatal("WorkerProfile = nil, want profile")
	}
	if pipeline.WorkerProfile.BotPool.Workers != 10 || pipeline.WorkerProfile.BotPool.QueueSize != 100 {
		t.Fatalf("BotPool = %#v, want 10/100", pipeline.WorkerProfile.BotPool)
	}
	if pipeline.WorkerProfile.Delivery.BreakerFailureThreshold != 5 || pipeline.Delivery.BreakerCooldownMs != 30000 {
		t.Fatalf("breaker fields not decoded: profile=%#v summary=%#v", pipeline.WorkerProfile.Delivery, pipeline.Delivery)
	}
}

func TestDecodeIrisBotWebhookPipelineDiagnosticsRejectsMissingProfile(t *testing.T) {
	raw := []byte(`{"workers":{"webhook":{"webhookPipeline":{"profileEnabled":true,"profileVersion":1,"profileId":"main","profileHash":"hash"}}}}`)

	_, err := DecodeIrisBotWebhookPipelineDiagnostics(raw)
	if !errors.Is(err, ErrRuntimeWorkerProfileDiagnosticsMissing) {
		t.Fatalf("DecodeIrisBotWebhookPipelineDiagnostics() error = %v, want ErrRuntimeWorkerProfileDiagnosticsMissing", err)
	}
}

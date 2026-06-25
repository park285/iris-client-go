package client

import (
	"errors"
	"fmt"
	"strings"

	"github.com/park285/iris-client-go/internal/jsonx"
)

var ErrRuntimeWorkerProfileDiagnosticsMissing = errors.New("iris: runtime worker profile diagnostics missing")

type RuntimeDiagnostics struct {
	Workers *RuntimeWorkersDiagnostics `json:"workers,omitempty"`
}

type RuntimeWorkersDiagnostics struct {
	Webhook RuntimeWorkerDiagnostics `json:"webhook"`
}

type RuntimeWorkerDiagnostics struct {
	WebhookPipeline *IrisBotWebhookPipelineDiagnostics `json:"webhookPipeline,omitempty"`
}

type IrisBotWebhookPipelineDiagnostics struct {
	ProfileEnabled  bool                          `json:"profileEnabled"`
	ProfileVersion  uint32                        `json:"profileVersion"`
	ProfileID       string                        `json:"profileId"`
	ProfileHash     string                        `json:"profileHash"`
	WorkerProfile   *IrisBotWebhookWorkerProfile  `json:"workerProfile,omitempty"`
	Delivery        IrisWebhookDeliveryDiagnostics `json:"delivery"`
	ReceiveExpected BotWebhookReceiveDiagnostics   `json:"receiveExpected"`
	BotPoolExpected BotPoolExpectedDiagnostics     `json:"botPoolExpected"`
}

type IrisWebhookDeliveryDiagnostics struct {
	WorkersConfigured      int    `json:"workersConfigured"`
	QueueCapacity          int    `json:"queueCapacity"`
	MaxGlobalInFlight      int    `json:"maxGlobalInFlight"`
	MaxPerEndpointInFlight int    `json:"maxPerEndpointInFlight"`
	MaxDrainPerTick        int    `json:"maxDrainPerTick"`
	MaxAttempts            uint32 `json:"maxAttempts"`
	RequestTimeoutMs       uint64 `json:"requestTimeoutMs"`
	LaneIdleTimeoutMs      uint64 `json:"laneIdleTimeoutMs"`
	BreakerFailureThreshold uint32 `json:"breakerFailureThreshold"`
	BreakerCooldownMs      uint64 `json:"breakerCooldownMs"`
}

type BotWebhookReceiveDiagnostics struct {
	WorkersExpected  int    `json:"workersExpected"`
	QueueSizeExpected int    `json:"queueSizeExpected"`
	EnqueueTimeoutMs uint64 `json:"enqueueTimeoutMs"`
	HandlerTimeoutMs uint64 `json:"handlerTimeoutMs"`
	MaxBodyBytes     uint64 `json:"maxBodyBytes"`
	DedupTTLMs       uint64 `json:"dedupTtlMs"`
	DedupTimeoutMs   uint64 `json:"dedupTimeoutMs"`
}

type BotPoolExpectedDiagnostics struct {
	WorkersExpected  int `json:"workersExpected"`
	QueueSizeExpected int `json:"queueSizeExpected"`
}

type IrisBotWebhookWorkerProfile struct {
	Version    uint32                                 `json:"version"`
	ProfileID  string                                 `json:"profile_id"`
	Delivery   IrisWebhookDeliveryWorkerProfile       `json:"delivery"`
	Receive    BotWebhookReceiveWorkerProfile         `json:"receive"`
	BotPool    BotPoolWorkerProfile                   `json:"bot_pool"`
	Validation IrisBotWebhookWorkerProfileValidation  `json:"validation"`
}

type IrisWebhookDeliveryWorkerProfile struct {
	LaneWorkers             int    `json:"lane_workers"`
	LaneQueueCapacity       int    `json:"lane_queue_capacity"`
	MaxGlobalInFlight       int    `json:"max_global_in_flight"`
	MaxPerEndpointInFlight  int    `json:"max_per_endpoint_in_flight"`
	MaxDrainPerTick         int    `json:"max_drain_per_tick"`
	MaxAttempts             uint32 `json:"max_attempts"`
	RequestTimeoutMs        uint64 `json:"request_timeout_ms"`
	LaneIdleTimeoutMs       uint64 `json:"lane_idle_timeout_ms"`
	BreakerFailureThreshold uint32 `json:"breaker_failure_threshold"`
	BreakerCooldownMs       uint64 `json:"breaker_cooldown_ms"`
}

type BotWebhookReceiveWorkerProfile struct {
	Workers          int    `json:"workers"`
	QueueSize        int    `json:"queue_size"`
	EnqueueTimeoutMs uint64 `json:"enqueue_timeout_ms"`
	HandlerTimeoutMs uint64 `json:"handler_timeout_ms"`
	MaxBodyBytes     uint64 `json:"max_body_bytes"`
	DedupTTLMs       uint64 `json:"dedup_ttl_ms"`
	DedupTimeoutMs   uint64 `json:"dedup_timeout_ms"`
}

type BotPoolWorkerProfile struct {
	Workers   int `json:"workers"`
	QueueSize int `json:"queue_size"`
}

type IrisBotWebhookWorkerProfileValidation struct {
	MinQueuePerEndpointMultiplier      int  `json:"min_queue_per_endpoint_multiplier"`
	RequireReceiveCapacityForEndpointBurst bool `json:"require_receive_capacity_for_endpoint_burst"`
}

func DecodeRuntimeDiagnostics(raw []byte) (*RuntimeDiagnostics, error) {
	var diagnostics RuntimeDiagnostics
	if err := jsonx.Unmarshal(raw, &diagnostics); err != nil {
		return nil, fmt.Errorf("decode Iris runtime diagnostics: %w", err)
	}
	return &diagnostics, nil
}

func DecodeIrisBotWebhookPipelineDiagnostics(raw []byte) (*IrisBotWebhookPipelineDiagnostics, error) {
	diagnostics, err := DecodeRuntimeDiagnostics(raw)
	if err != nil {
		return nil, err
	}
	if diagnostics == nil || diagnostics.Workers == nil || diagnostics.Workers.Webhook.WebhookPipeline == nil {
		return nil, ErrRuntimeWorkerProfileDiagnosticsMissing
	}
	pipeline := diagnostics.Workers.Webhook.WebhookPipeline
	if err := validateIrisBotWebhookPipelineDiagnostics(pipeline); err != nil {
		return nil, err
	}
	return pipeline, nil
}

func validateIrisBotWebhookPipelineDiagnostics(pipeline *IrisBotWebhookPipelineDiagnostics) error {
	if pipeline == nil {
		return ErrRuntimeWorkerProfileDiagnosticsMissing
	}
	if pipeline.WorkerProfile == nil {
		return fmt.Errorf("%w: workers.webhook.webhookPipeline.workerProfile is missing", ErrRuntimeWorkerProfileDiagnosticsMissing)
	}
	if pipeline.ProfileVersion == 0 || pipeline.WorkerProfile.Version == 0 {
		return fmt.Errorf("%w: worker profile version is missing", ErrRuntimeWorkerProfileDiagnosticsMissing)
	}
	if strings.TrimSpace(pipeline.ProfileHash) == "" {
		return fmt.Errorf("%w: profileHash is missing", ErrRuntimeWorkerProfileDiagnosticsMissing)
	}
	if strings.TrimSpace(pipeline.ProfileID) == "" || strings.TrimSpace(pipeline.WorkerProfile.ProfileID) == "" {
		return fmt.Errorf("%w: profile id is missing", ErrRuntimeWorkerProfileDiagnosticsMissing)
	}
	return nil
}

package models

import (
	"time"

	"github.com/google/uuid"
)

type WebhookEvent struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	Provider        string     `db:"provider" json:"provider"`
	ProviderEventID *string    `db:"provider_event_id" json:"provider_event_id,omitempty"`
	PayloadHash     *string    `db:"payload_hash" json:"payload_hash,omitempty"`
	EventType       string     `db:"event_type" json:"event_type"`
	Payload         JSONB      `db:"payload" json:"payload"`
	SignatureValid  *bool      `db:"signature_valid" json:"signature_valid,omitempty"`
	RelatedCallID   *uuid.UUID `db:"related_call_id" json:"related_call_id,omitempty"`
	ReceivedAt      time.Time  `db:"received_at" json:"received_at"`
	ProcessedAt     *time.Time `db:"processed_at" json:"processed_at,omitempty"`
	Status          string     `db:"status" json:"status"`
	ErrorMessage    *string    `db:"error_message" json:"error_message,omitempty"`
	AttemptCount    int        `db:"attempt_count" json:"attempt_count"`
	MaxAttempts     int        `db:"max_attempts" json:"max_attempts"`
	NextRetryAt     *time.Time `db:"next_retry_at" json:"next_retry_at,omitempty"`
	LockedAt        *time.Time `db:"locked_at" json:"locked_at,omitempty"`
	LockedBy        *string    `db:"locked_by" json:"locked_by,omitempty"`
	LockToken       *uuid.UUID `db:"lock_token" json:"lock_token,omitempty"`
	LockExpiresAt   *time.Time `db:"lock_expires_at" json:"lock_expires_at,omitempty"`
}

type APIUsageLog struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	CallID            *uuid.UUID `db:"call_id" json:"call_id,omitempty"`
	Provider          string     `db:"provider" json:"provider"`
	Operation         string     `db:"operation" json:"operation"`
	RequestID         *uuid.UUID `db:"request_id" json:"request_id,omitempty"`
	ProviderRequestID *string    `db:"provider_request_id" json:"provider_request_id,omitempty"`
	TraceID           *uuid.UUID `db:"trace_id" json:"trace_id,omitempty"`
	UnitsConsumed     *float64   `db:"units_consumed" json:"units_consumed,omitempty"`
	UnitType          *string    `db:"unit_type" json:"unit_type,omitempty"`
	CostUSD           *float64   `db:"cost_usd" json:"cost_usd,omitempty"`
	Metadata          JSONB      `db:"metadata" json:"metadata"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
}

type SystemSetting struct {
	Key       string    `db:"key" json:"key"`
	Value     JSONB     `db:"value" json:"value"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

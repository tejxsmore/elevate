package models

import (
	"time"

	"github.com/google/uuid"
)

type CallAction struct {
	ID               uuid.UUID     `db:"id" json:"id"`
	CallID           uuid.UUID     `db:"call_id" json:"call_id"`
	ActionType       ActionType    `db:"action_type" json:"action_type"`
	Status           ActionStatus  `db:"status" json:"status"`
	Trigger          ActionTrigger `db:"trigger" json:"trigger"`
	TriggerSegmentID *uuid.UUID    `db:"trigger_segment_id" json:"trigger_segment_id,omitempty"`
	Payload          JSONB         `db:"payload" json:"payload"`
	IdempotencyKey   *string       `db:"idempotency_key" json:"idempotency_key,omitempty"`
	AvailableAt      time.Time     `db:"available_at" json:"available_at"`
	AttemptCount     int           `db:"attempt_count" json:"attempt_count"`
	MaxAttempts      int           `db:"max_attempts" json:"max_attempts"`
	LockedAt         *time.Time    `db:"locked_at" json:"locked_at,omitempty"`
	LockedBy         *string       `db:"locked_by" json:"locked_by,omitempty"`
	LockToken        *uuid.UUID    `db:"lock_token" json:"lock_token,omitempty"`
	LockExpiresAt    *time.Time    `db:"lock_expires_at" json:"lock_expires_at,omitempty"`
	Priority         int16         `db:"priority" json:"priority"`
	LastError        JSONB         `db:"last_error" json:"last_error,omitempty"`
	TriggeredAt      time.Time     `db:"triggered_at" json:"triggered_at"`
	CompletedAt      *time.Time    `db:"completed_at" json:"completed_at,omitempty"`
	LatencyMs        *int          `db:"latency_ms" json:"latency_ms,omitempty"`
	ErrorMessage     *string       `db:"error_message" json:"error_message,omitempty"`
}

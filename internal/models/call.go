package models

import (
	"time"

	"github.com/google/uuid"
)

type Call struct {
	ID                           uuid.UUID           `db:"id" json:"id"`
	LeadID                       uuid.UUID           `db:"lead_id" json:"lead_id"`
	CampaignID                   *uuid.UUID          `db:"campaign_id" json:"campaign_id,omitempty"`
	CampaignVersion              *int                `db:"campaign_version" json:"campaign_version,omitempty"`
	ParentCallID                 *uuid.UUID          `db:"parent_call_id" json:"parent_call_id,omitempty"`
	Provider                     string              `db:"provider" json:"provider"`
	ProviderCallID               *string             `db:"provider_call_id" json:"provider_call_id,omitempty"`
	Direction                    CallDirection       `db:"direction" json:"direction"`
	Status                       CallStatus          `db:"status" json:"status"`
	AttemptNumber                int                 `db:"attempt_number" json:"attempt_number"`
	PrimaryLanguage              LanguageCode        `db:"primary_language" json:"primary_language"`
	CurrentClassification        ClassificationLabel `db:"current_classification" json:"current_classification"`
	ClassificationConfidence     *float64            `db:"classification_confidence" json:"classification_confidence,omitempty"`
	ClassificationSequenceNumber *int                `db:"classification_sequence_number" json:"classification_sequence_number,omitempty"`
	QueuedAt                     time.Time           `db:"queued_at" json:"queued_at"`
	ScheduledFor                 *time.Time          `db:"scheduled_for" json:"scheduled_for,omitempty"`
	DialedAt                     *time.Time          `db:"dialed_at" json:"dialed_at,omitempty"`
	AnsweredAt                   *time.Time          `db:"answered_at" json:"answered_at,omitempty"`
	EndedAt                      *time.Time          `db:"ended_at" json:"ended_at,omitempty"`
	DurationSeconds              *int                `db:"duration_seconds" json:"duration_seconds,omitempty"`
	EndedReason                  *string             `db:"ended_reason" json:"ended_reason,omitempty"`
	FailureCode                  *string             `db:"failure_code" json:"failure_code,omitempty"`
	ProviderErrorCode            *string             `db:"provider_error_code" json:"provider_error_code,omitempty"`
	RetryAfter                   *time.Time          `db:"retry_after" json:"retry_after,omitempty"`
	RecordingURL                 *string             `db:"recording_url" json:"recording_url,omitempty"`
	RecordingSID                 *string             `db:"recording_sid" json:"recording_sid,omitempty"`
	TwimlStreamSID               *string             `db:"twiml_stream_sid" json:"twiml_stream_sid,omitempty"`
	CreatedAt                    time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt                    time.Time           `db:"updated_at" json:"updated_at"`
}

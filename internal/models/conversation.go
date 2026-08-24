package models

import (
	"time"

	"github.com/google/uuid"
)

type ConversationTurn struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	CallID            uuid.UUID  `db:"call_id" json:"call_id"`
	SequenceNumber    int        `db:"sequence_number" json:"sequence_number"`
	ExtractedContext  JSONB      `db:"extracted_context" json:"extracted_context"`
	Model             *string    `db:"model" json:"model,omitempty"`
	ProviderRequestID *string    `db:"provider_request_id" json:"provider_request_id,omitempty"`
	StartedAt         *time.Time `db:"started_at" json:"started_at,omitempty"`
	CompletedAt       *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	LatencyMs         *int       `db:"latency_ms" json:"latency_ms,omitempty"`
}

type CallTranscriptSegment struct {
	ID                uuid.UUID    `db:"id" json:"id"`
	CallID            uuid.UUID    `db:"call_id" json:"call_id"`
	TurnID            *uuid.UUID   `db:"turn_id" json:"turn_id,omitempty"`
	SegmentSequence   int          `db:"segment_sequence" json:"segment_sequence"`
	Revision          int          `db:"revision" json:"revision"`
	STTProvider       string       `db:"stt_provider" json:"stt_provider"`
	ProviderSegmentID *string      `db:"provider_segment_id" json:"provider_segment_id,omitempty"`
	Speaker           SpeakerRole  `db:"speaker" json:"speaker"`
	Text              string       `db:"text" json:"text"`
	LanguageDetected  LanguageCode `db:"language_detected" json:"language_detected"`
	DetectedLanguages StringArray  `db:"detected_languages" json:"detected_languages"`
	Confidence        *float64     `db:"confidence" json:"confidence,omitempty"`
	IsFinal           bool         `db:"is_final" json:"is_final"`
	IsInterrupted     bool         `db:"is_interrupted" json:"is_interrupted"`
	StartedAtMs       *int         `db:"started_at_ms" json:"started_at_ms,omitempty"`
	EndedAtMs         *int         `db:"ended_at_ms" json:"ended_at_ms,omitempty"`
	ProviderRequestID *string      `db:"provider_request_id" json:"provider_request_id,omitempty"`
	CreatedAt         time.Time    `db:"created_at" json:"created_at"`
}

type CallMessage struct {
	ID                uuid.UUID   `db:"id" json:"id"`
	CallID            uuid.UUID   `db:"call_id" json:"call_id"`
	TurnID            *uuid.UUID  `db:"turn_id" json:"turn_id,omitempty"`
	SequenceNumber    int         `db:"sequence_number" json:"sequence_number"`
	Role              MessageRole `db:"role" json:"role"`
	Content           string      `db:"content" json:"content"`
	Input             JSONB       `db:"input" json:"input,omitempty"`
	Output            JSONB       `db:"output" json:"output,omitempty"`
	ToolName          *string     `db:"tool_name" json:"tool_name,omitempty"`
	ToolCallID        *string     `db:"tool_call_id" json:"tool_call_id,omitempty"`
	FinishReason      *string     `db:"finish_reason" json:"finish_reason,omitempty"`
	ProviderRequestID *string     `db:"provider_request_id" json:"provider_request_id,omitempty"`
	Model             *string     `db:"model" json:"model,omitempty"`
	PromptTokens      *int        `db:"prompt_tokens" json:"prompt_tokens,omitempty"`
	CompletionTokens  *int        `db:"completion_tokens" json:"completion_tokens,omitempty"`
	LatencyMs         *int        `db:"latency_ms" json:"latency_ms,omitempty"`
	CreatedAt         time.Time   `db:"created_at" json:"created_at"`
}

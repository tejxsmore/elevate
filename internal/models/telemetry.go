package models

import (
	"time"

	"github.com/google/uuid"
)

type CallEvent struct {
	ID         uuid.UUID `db:"id" json:"id"`
	CallID     uuid.UUID `db:"call_id" json:"call_id"`
	EventType  string    `db:"event_type" json:"event_type"`
	EventData  JSONB     `db:"event_data" json:"event_data"`
	OccurredAt time.Time `db:"occurred_at" json:"occurred_at"`
}

type LatencyMetric struct {
	ID                 uuid.UUID    `db:"id" json:"id"`
	CallID             uuid.UUID    `db:"call_id" json:"call_id"`
	TurnSequenceNumber *int         `db:"turn_sequence_number" json:"turn_sequence_number,omitempty"`
	Stage              LatencyStage `db:"stage" json:"stage"`
	DurationMs         int          `db:"duration_ms" json:"duration_ms"`
	MeasuredAt         time.Time    `db:"measured_at" json:"measured_at"`
}

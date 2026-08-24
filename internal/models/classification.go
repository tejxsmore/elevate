package models

import (
	"time"

	"github.com/google/uuid"
)

type LeadClassification struct {
	ID                    uuid.UUID           `db:"id" json:"id"`
	CallID                uuid.UUID           `db:"call_id" json:"call_id"`
	SequenceNumber        int                 `db:"sequence_number" json:"sequence_number"`
	Classification        ClassificationLabel `db:"classification" json:"classification"`
	Confidence            *float64            `db:"confidence" json:"confidence,omitempty"`
	ClassificationSummary *string             `db:"classification_summary" json:"classification_summary,omitempty"`
	Signals               JSONB               `db:"signals" json:"signals"`
	TriggeringSegmentID   *uuid.UUID          `db:"triggering_segment_id" json:"triggering_segment_id,omitempty"`
	ClassifiedAt          time.Time           `db:"classified_at" json:"classified_at"`
}

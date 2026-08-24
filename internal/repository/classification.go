package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"elevate/internal/database"
	"elevate/internal/models"
)

type ClassificationRepo struct {
	db *database.DB
}

func NewClassificationRepo(db *database.DB) *ClassificationRepo {
	return &ClassificationRepo{db: db}
}

func (r *ClassificationRepo) Create(
	ctx context.Context,
	callID uuid.UUID,
	classification models.ClassificationLabel,
	confidence float64,
	summary string,
	signals map[string]any,
	triggeringSegmentID *uuid.UUID,
) (models.LeadClassification, error) {
	rawSignals, err := json.Marshal(signals)
	if err != nil {
		return models.LeadClassification{}, err
	}

	var item models.LeadClassification

	err = r.db.Pool.QueryRow(
		ctx,
		`
		WITH next_sequence AS (
			SELECT COALESCE(MAX(sequence_number) + 1, 0) AS sequence_number
			FROM lead_classifications
			WHERE call_id = $1
		)
		INSERT INTO lead_classifications (
			call_id,
			sequence_number,
			classification,
			confidence,
			classification_summary,
			signals,
			triggering_segment_id
		)
		SELECT
			$1,
			next_sequence.sequence_number,
			$2,
			$3,
			$4,
			$5,
			$6
		FROM next_sequence
		RETURNING
			id,
			call_id,
			sequence_number,
			classification,
			confidence,
			classification_summary,
			signals,
			triggering_segment_id,
			classified_at
		`,
		callID,
		classification,
		confidence,
		summary,
		rawSignals,
		triggeringSegmentID,
	).Scan(
		&item.ID,
		&item.CallID,
		&item.SequenceNumber,
		&item.Classification,
		&item.Confidence,
		&item.ClassificationSummary,
		&item.Signals,
		&item.TriggeringSegmentID,
		&item.ClassifiedAt,
	)

	return item, err
}

func (r *ClassificationRepo) Latest(
	ctx context.Context,
	callID uuid.UUID,
) (models.LeadClassification, error) {
	var item models.LeadClassification

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			sequence_number,
			classification,
			confidence,
			classification_summary,
			signals,
			triggering_segment_id,
			classified_at
		FROM lead_classifications
		WHERE call_id = $1
		ORDER BY sequence_number DESC
		LIMIT 1
		`,
		callID,
	).Scan(
		&item.ID,
		&item.CallID,
		&item.SequenceNumber,
		&item.Classification,
		&item.Confidence,
		&item.ClassificationSummary,
		&item.Signals,
		&item.TriggeringSegmentID,
		&item.ClassifiedAt,
	)

	return item, err
}

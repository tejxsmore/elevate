package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"elevate/internal/database"
	"elevate/internal/models"
)

type CallRepo struct {
	db *database.DB
}

func NewCallRepo(db *database.DB) *CallRepo {
	return &CallRepo{db: db}
}

const callColumns = `
	id,
	lead_id,
	campaign_id,
	campaign_version,
	parent_call_id,
	provider,
	provider_call_id,
	direction,
	status,
	attempt_number,
	primary_language,
	current_classification,
	classification_confidence,
	classification_sequence_number,
	queued_at,
	scheduled_for,
	dialed_at,
	answered_at,
	ended_at,
	duration_seconds,
	ended_reason,
	failure_code,
	provider_error_code,
	retry_after,
	recording_url,
	recording_sid,
	twiml_stream_sid,
	created_at,
	updated_at
`

func scanCall(
	row interface {
		Scan(dest ...any) error
	},
	call *models.Call,
) error {
	return row.Scan(
		&call.ID,
		&call.LeadID,
		&call.CampaignID,
		&call.CampaignVersion,
		&call.ParentCallID,
		&call.Provider,
		&call.ProviderCallID,
		&call.Direction,
		&call.Status,
		&call.AttemptNumber,
		&call.PrimaryLanguage,
		&call.CurrentClassification,
		&call.ClassificationConfidence,
		&call.ClassificationSequenceNumber,
		&call.QueuedAt,
		&call.ScheduledFor,
		&call.DialedAt,
		&call.AnsweredAt,
		&call.EndedAt,
		&call.DurationSeconds,
		&call.EndedReason,
		&call.FailureCode,
		&call.ProviderErrorCode,
		&call.RetryAfter,
		&call.RecordingURL,
		&call.RecordingSID,
		&call.TwimlStreamSID,
		&call.CreatedAt,
		&call.UpdatedAt,
	)
}

func (r *CallRepo) Create(
	ctx context.Context,
	leadID uuid.UUID,
	campaignID uuid.UUID,
) (models.Call, error) {
	var call models.Call

	row := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO calls (
			lead_id,
			campaign_id,
			campaign_version,
			provider,
			direction,
			status,
			primary_language
		)
		SELECT
			$1,
			c.id,
			c.version,
			'twilio',
			'outbound',
			'queued',
			l.preferred_language
		FROM campaigns c
		JOIN leads l
			ON l.id = $1
		WHERE c.id = $2
		  AND c.active = true
		  AND c.archived_at IS NULL
		RETURNING `+callColumns,
		leadID,
		campaignID,
	)

	if err := scanCall(row, &call); err != nil {
		return models.Call{}, err
	}

	return call, nil
}

func (r *CallRepo) Get(
	ctx context.Context,
	id uuid.UUID,
) (models.Call, error) {
	var call models.Call

	row := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT `+callColumns+`
		FROM calls
		WHERE id = $1
		`,
		id,
	)

	if err := scanCall(row, &call); err != nil {
		return models.Call{}, err
	}

	return call, nil
}

type CallSummary struct {
	ID                    uuid.UUID                  `json:"id"`
	LeadID                uuid.UUID                  `json:"lead_id"`
	CampaignID            *uuid.UUID                 `json:"campaign_id,omitempty"`
	Status                models.CallStatus          `json:"status"`
	CurrentClassification models.ClassificationLabel `json:"current_classification"`
	PrimaryLanguage       models.LanguageCode        `json:"primary_language"`
	Direction             models.CallDirection       `json:"direction"`
	AttemptNumber         int                        `json:"attempt_number"`
	QueuedAt              time.Time                  `json:"queued_at"`
	DialedAt              *time.Time                 `json:"dialed_at,omitempty"`
	AnsweredAt            *time.Time                 `json:"answered_at,omitempty"`
	EndedAt               *time.Time                 `json:"ended_at,omitempty"`
	CreatedAt             time.Time                  `json:"created_at"`
}

func (r *CallRepo) List(
	ctx context.Context,
	limit int,
	offset int,
) ([]CallSummary, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			lead_id,
			campaign_id,
			status,
			current_classification,
			primary_language,
			direction,
			attempt_number,
			queued_at,
			dialed_at,
			answered_at,
			ended_at,
			created_at
		FROM calls
		ORDER BY created_at DESC
		LIMIT $1
		OFFSET $2
		`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	calls := make(
		[]CallSummary,
		0,
		limit,
	)

	for rows.Next() {
		var call CallSummary

		if err := rows.Scan(
			&call.ID,
			&call.LeadID,
			&call.CampaignID,
			&call.Status,
			&call.CurrentClassification,
			&call.PrimaryLanguage,
			&call.Direction,
			&call.AttemptNumber,
			&call.QueuedAt,
			&call.DialedAt,
			&call.AnsweredAt,
			&call.EndedAt,
			&call.CreatedAt,
		); err != nil {
			return nil, err
		}

		calls = append(
			calls,
			call,
		)
	}

	return calls, rows.Err()
}

func (r *CallRepo) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	failureCode string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			status = 'failed',
			failure_code = $1,
			ended_at = COALESCE(ended_at, now()),
			ended_reason = COALESCE(ended_reason, $1)
		WHERE id = $2
		`,
		failureCode,
		id,
	)

	return err
}

func (r *CallRepo) SetDialing(
	ctx context.Context,
	id uuid.UUID,
	providerCallID string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			provider_call_id = COALESCE(
				NULLIF(provider_call_id, ''),
				$1
			),
			status = CASE
				WHEN status = 'queued'
				THEN 'dialing'
				ELSE status
			END,
			dialed_at = COALESCE(
				dialed_at,
				now()
			)
		WHERE id = $2
		`,
		providerCallID,
		id,
	)

	return err
}

func (r *CallRepo) UpdateStatusByProviderCallID(
	ctx context.Context,
	providerCallID string,
	status models.CallStatus,
	duration *int,
) (uuid.UUID, error) {
	var callID uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		UPDATE calls
		SET
			status = $1,
			duration_seconds = COALESCE(
				$2,
				duration_seconds
			),
			answered_at = CASE
				WHEN $1 = 'in_progress'
				THEN COALESCE(
					answered_at,
					now()
				)
				ELSE answered_at
			END,
			ended_at = CASE
				WHEN $1 IN (
					'completed',
					'failed',
					'no_answer',
					'busy',
					'canceled'
				)
				THEN COALESCE(
					ended_at,
					now()
				)
				ELSE ended_at
			END
		WHERE provider_call_id = $3
		RETURNING id
		`,
		status,
		duration,
		providerCallID,
	).Scan(&callID)

	return callID, err
}

func (r *CallRepo) UpdateStatusByCallID(
	ctx context.Context,
	callID uuid.UUID,
	providerCallID string,
	status models.CallStatus,
	duration *int,
) (uuid.UUID, error) {
	var updatedID uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		UPDATE calls
		SET
			provider_call_id = CASE
				WHEN NULLIF($2, '') IS NOT NULL
				THEN COALESCE(
					provider_call_id,
					$2
				)
				ELSE provider_call_id
			END,
			status = $3,
			duration_seconds = COALESCE(
				$4,
				duration_seconds
			),
			answered_at = CASE
				WHEN $3 = 'in_progress'
				THEN COALESCE(
					answered_at,
					now()
				)
				ELSE answered_at
			END,
			ended_at = CASE
				WHEN $3 IN (
					'completed',
					'failed',
					'no_answer',
					'busy',
					'canceled'
				)
				THEN COALESCE(
					ended_at,
					now()
				)
				ELSE ended_at
			END
		WHERE id = $1
		RETURNING id
		`,
		callID,
		providerCallID,
		status,
		duration,
	).Scan(&updatedID)

	return updatedID, err
}

func (r *CallRepo) Transcript(
	ctx context.Context,
	callID uuid.UUID,
) ([]models.CallTranscriptSegment, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT DISTINCT ON (segment_sequence)
			id,
			call_id,
			turn_id,
			segment_sequence,
			revision,
			stt_provider,
			speaker,
			text,
			language_detected,
			detected_languages,
			confidence,
			is_final,
			is_interrupted,
			started_at_ms,
			ended_at_ms,
			created_at
		FROM call_transcript_segments
		WHERE call_id = $1
		ORDER BY
			segment_sequence,
			revision DESC
		`,
		callID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	segments := make(
		[]models.CallTranscriptSegment,
		0,
	)

	for rows.Next() {
		var segment models.CallTranscriptSegment

		if err := rows.Scan(
			&segment.ID,
			&segment.CallID,
			&segment.TurnID,
			&segment.SegmentSequence,
			&segment.Revision,
			&segment.STTProvider,
			&segment.Speaker,
			&segment.Text,
			&segment.LanguageDetected,
			&segment.DetectedLanguages,
			&segment.Confidence,
			&segment.IsFinal,
			&segment.IsInterrupted,
			&segment.StartedAtMs,
			&segment.EndedAtMs,
			&segment.CreatedAt,
		); err != nil {
			return nil, err
		}

		segments = append(
			segments,
			segment,
		)
	}

	return segments, rows.Err()
}

type ActionSummary struct {
	ID           uuid.UUID            `json:"id"`
	CallID       uuid.UUID            `json:"call_id"`
	ActionType   models.ActionType    `json:"action_type"`
	Status       models.ActionStatus  `json:"status"`
	Trigger      models.ActionTrigger `json:"trigger"`
	Payload      models.JSONB         `json:"payload"`
	AttemptCount int                  `json:"attempt_count"`
	MaxAttempts  int                  `json:"max_attempts"`
	Priority     int16                `json:"priority"`
	TriggeredAt  time.Time            `json:"triggered_at"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty"`
	ErrorMessage *string              `json:"error_message,omitempty"`
}

func (r *CallRepo) Actions(
	ctx context.Context,
	callID uuid.UUID,
) ([]ActionSummary, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			call_id,
			action_type,
			status,
			trigger,
			payload,
			attempt_count,
			max_attempts,
			priority,
			triggered_at,
			completed_at,
			error_message
		FROM call_actions
		WHERE call_id = $1
		ORDER BY triggered_at ASC
		`,
		callID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	actions := make(
		[]ActionSummary,
		0,
	)

	for rows.Next() {
		var action ActionSummary

		if err := rows.Scan(
			&action.ID,
			&action.CallID,
			&action.ActionType,
			&action.Status,
			&action.Trigger,
			&action.Payload,
			&action.AttemptCount,
			&action.MaxAttempts,
			&action.Priority,
			&action.TriggeredAt,
			&action.CompletedAt,
			&action.ErrorMessage,
		); err != nil {
			return nil, err
		}

		actions = append(
			actions,
			action,
		)
	}

	return actions, rows.Err()
}

func (r *CallRepo) InsertEvent(
	ctx context.Context,
	callID uuid.UUID,
	eventType string,
	eventData []byte,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		INSERT INTO call_events (
			call_id,
			event_type,
			event_data
		)
		VALUES (
			$1,
			$2,
			$3
		)
		`,
		callID,
		eventType,
		eventData,
	)

	return err
}

func (r *CallRepo) UpdateRecording(
	ctx context.Context,
	callID uuid.UUID,
	recordingSID string,
	recordingURL string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			recording_sid = $2,
			recording_url = $3
		WHERE id = $1
		`,
		callID,
		recordingSID,
		recordingURL,
	)

	return err
}

func (r *CallRepo) SetClassification(
	ctx context.Context,
	id uuid.UUID,
	classification models.ClassificationLabel,
	confidence float64,
	sequenceNumber int,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			current_classification = $2,
			classification_confidence = $3,
			classification_sequence_number = $4
		WHERE id = $1
		`,
		id,
		classification,
		confidence,
		sequenceNumber,
	)

	return err
}

package repository

import (
	"context"

	"github.com/google/uuid"

	"elevate/internal/database"
	"elevate/internal/models"
)

type ConversationRepo struct {
	db *database.DB
}

func NewConversationRepo(
	db *database.DB,
) *ConversationRepo {
	return &ConversationRepo{
		db: db,
	}
}

type ConversationMessage struct {
	Role    models.MessageRole
	Content string
}

func (r *ConversationRepo) StartTurn(
	ctx context.Context,
	callID uuid.UUID,
	sequenceNumber int,
) (uuid.UUID, error) {
	var turnID uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO conversation_turns (
			call_id,
			sequence_number,
			started_at
		)
		VALUES ($1, $2, now())
		ON CONFLICT (
			call_id,
			sequence_number
		) DO UPDATE
		SET started_at = COALESCE(
			conversation_turns.started_at,
			EXCLUDED.started_at
		)
		RETURNING id
		`,
		callID,
		sequenceNumber,
	).Scan(&turnID)

	return turnID, err
}

func (r *ConversationRepo) CompleteTurn(
	ctx context.Context,
	turnID uuid.UUID,
	latencyMs *int,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE conversation_turns
		SET
			completed_at = now(),
			latency_ms = COALESCE($2, latency_ms)
		WHERE id = $1
		`,
		turnID,
		latencyMs,
	)

	return err
}

func (r *ConversationRepo) InsertTranscriptSegment(
	ctx context.Context,
	callID uuid.UUID,
	turnID *uuid.UUID,
	segmentSequence int,
	speaker models.SpeakerRole,
	text string,
	language models.LanguageCode,
	detectedLanguages []string,
	confidence *float64,
	isFinal bool,
	startedMs *int,
	endedMs *int,
) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO call_transcript_segments (
			call_id,
			turn_id,
			segment_sequence,
			speaker,
			text,
			language_detected,
			detected_languages,
			confidence,
			is_final,
			started_at_ms,
			ended_at_ms
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11
		)
		ON CONFLICT (
			call_id,
			segment_sequence,
			revision
		) DO UPDATE
		SET
			text = EXCLUDED.text,
			language_detected = EXCLUDED.language_detected,
			detected_languages = EXCLUDED.detected_languages,
			is_final = EXCLUDED.is_final,
			confidence = EXCLUDED.confidence,
			started_at_ms = EXCLUDED.started_at_ms,
			ended_at_ms = EXCLUDED.ended_at_ms
		RETURNING id
		`,
		callID,
		turnID,
		segmentSequence,
		speaker,
		text,
		language,
		detectedLanguages,
		confidence,
		isFinal,
		startedMs,
		endedMs,
	).Scan(&id)

	return id, err
}

func (r *ConversationRepo) InsertCallMessage(
	ctx context.Context,
	callID uuid.UUID,
	turnID *uuid.UUID,
	sequenceNumber int,
	role models.MessageRole,
	content string,
) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO call_messages (
			call_id,
			turn_id,
			sequence_number,
			role,
			content
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (
			call_id,
			sequence_number
		) DO UPDATE
		SET
			turn_id = COALESCE(
				call_messages.turn_id,
				EXCLUDED.turn_id
			),
			role = EXCLUDED.role,
			content = EXCLUDED.content
		RETURNING id
		`,
		callID,
		turnID,
		sequenceNumber,
		role,
		content,
	).Scan(&id)

	return id, err
}

func (r *ConversationRepo) RecentMessages(
	ctx context.Context,
	callID uuid.UUID,
	limit int,
) ([]ConversationMessage, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			role,
			content
		FROM (
			SELECT
				role,
				content,
				sequence_number
			FROM call_messages
			WHERE call_id = $1
			ORDER BY sequence_number DESC
			LIMIT $2
		) recent
		ORDER BY sequence_number ASC
		`,
		callID,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	messages := make(
		[]ConversationMessage,
		0,
		limit,
	)

	for rows.Next() {
		var message ConversationMessage

		if err := rows.Scan(
			&message.Role,
			&message.Content,
		); err != nil {
			return nil, err
		}

		messages = append(
			messages,
			message,
		)
	}

	return messages, rows.Err()
}

func (r *ConversationRepo) InsertLatencyMetric(
	ctx context.Context,
	callID uuid.UUID,
	turnSequence *int,
	stage models.LatencyStage,
	durationMs int,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		INSERT INTO latency_metrics (
			call_id,
			turn_sequence_number,
			stage,
			duration_ms
		)
		VALUES ($1, $2, $3, $4)
		`,
		callID,
		turnSequence,
		stage,
		durationMs,
	)

	return err
}

func (r *ConversationRepo) UpdateCallPrimaryLanguage(
	ctx context.Context,
	callID uuid.UUID,
	language models.LanguageCode,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET primary_language = $2
		WHERE id = $1
		`,
		callID,
		language,
	)

	return err
}

func (r *ConversationRepo) MarkCallInProgress(
	ctx context.Context,
	callID uuid.UUID,
	streamSID string,
	callSID string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			status = CASE
				WHEN status IN (
					'queued',
					'dialing',
					'ringing',
					'in_progress'
				)
				THEN 'in_progress'
				ELSE status
			END,
			provider_call_id = COALESCE(
				provider_call_id,
				NULLIF($3, '')
			),
			answered_at = CASE
				WHEN status IN (
					'queued',
					'dialing',
					'ringing',
					'in_progress'
				)
				THEN COALESCE(answered_at, now())
				ELSE answered_at
			END,
			twiml_stream_sid = COALESCE(
				NULLIF($2, ''),
				twiml_stream_sid
			)
		WHERE id = $1
		`,
		callID,
		streamSID,
		callSID,
	)

	return err
}

func (r *ConversationRepo) MarkCallEnded(
	ctx context.Context,
	callID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE calls
		SET
			status = CASE
				WHEN status = 'in_progress'
				THEN 'completed'
				ELSE status
			END,
			ended_at = CASE
				WHEN status IN (
					'in_progress',
					'completed',
					'failed',
					'busy',
					'no_answer',
					'canceled'
				)
				THEN COALESCE(ended_at, now())
				ELSE ended_at
			END
		WHERE id = $1
		`,
		callID,
	)

	return err
}

type CallContext struct {
	CallID            uuid.UUID
	LeadID            uuid.UUID
	PreferredLanguage models.LanguageCode
	SystemPrompt      *string
	VoiceConfig       models.JSONB
}

func (r *ConversationRepo) GetCallContext(
	ctx context.Context,
	callID uuid.UUID,
) (*CallContext, error) {
	cc := &CallContext{
		CallID: callID,
	}

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			c.lead_id,
			l.preferred_language,
			cv.system_prompt,
			cv.voice_config
		FROM calls c
		JOIN leads l
			ON l.id = c.lead_id
		LEFT JOIN campaign_versions cv
			ON cv.campaign_id = c.campaign_id
			AND cv.version = c.campaign_version
		WHERE c.id = $1
		`,
		callID,
	).Scan(
		&cc.LeadID,
		&cc.PreferredLanguage,
		&cc.SystemPrompt,
		&cc.VoiceConfig,
	)

	if err != nil {
		return nil, err
	}

	return cc, nil
}

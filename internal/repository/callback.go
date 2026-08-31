package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
	"elevate/internal/models"
)

type CallbackRepo struct {
	db *database.DB
}

func NewCallbackRepo(
	db *database.DB,
) *CallbackRepo {
	return &CallbackRepo{
		db: db,
	}
}

type CallbackSummary struct {
	ID                uuid.UUID             `json:"id"`
	CallID            uuid.UUID             `json:"call_id"`
	LeadID            uuid.UUID             `json:"lead_id"`
	RequestedTimeText string                `json:"requested_time_text"`
	ScheduledFor      *time.Time            `json:"scheduled_for,omitempty"`
	Timezone          string                `json:"timezone"`
	Status            models.CallbackStatus `json:"status"`
	ReminderSent      bool                  `json:"reminder_sent"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

type CreateCallbackInput struct {
	CallID               uuid.UUID
	LeadID               uuid.UUID
	RequestedTimeText    string
	ScheduledFor         *time.Time
	Timezone             string
	ResolutionConfidence *float64
	ResolutionSource     string
	ResolvedFrom         []byte
	Status               models.CallbackStatus
	CallbackActionID     *uuid.UUID
}

type ClaimedCallback struct {
	Callback models.ScheduledCallback
	Call     models.Call
}

func (r *CallbackRepo) Create(
	ctx context.Context,
	in CreateCallbackInput,
) (models.ScheduledCallback, error) {
	var callback models.ScheduledCallback

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO scheduled_callbacks (
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			resolution_confidence,
			resolution_source,
			resolved_from,
			status,
			callback_action_id
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
			$10
		)
		RETURNING
			id,
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			resolution_confidence,
			resolution_source,
			resolved_from,
			status,
			reminder_sent,
			callback_action_id,
			follow_up_call_id,
			created_at,
			updated_at
		`,
		in.CallID,
		in.LeadID,
		in.RequestedTimeText,
		in.ScheduledFor,
		in.Timezone,
		in.ResolutionConfidence,
		in.ResolutionSource,
		in.ResolvedFrom,
		in.Status,
		in.CallbackActionID,
	).Scan(
		&callback.ID,
		&callback.CallID,
		&callback.LeadID,
		&callback.RequestedTimeText,
		&callback.ScheduledFor,
		&callback.Timezone,
		&callback.ResolutionConfidence,
		&callback.ResolutionSource,
		&callback.ResolvedFrom,
		&callback.Status,
		&callback.ReminderSent,
		&callback.CallbackActionID,
		&callback.FollowUpCallID,
		&callback.CreatedAt,
		&callback.UpdatedAt,
	)

	return callback, err
}

func (r *CallbackRepo) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (models.ScheduledCallback, error) {
	var callback models.ScheduledCallback

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			resolution_confidence,
			resolution_source,
			resolved_from,
			status,
			reminder_sent,
			callback_action_id,
			follow_up_call_id,
			created_at,
			updated_at
		FROM scheduled_callbacks
		WHERE id = $1
		`,
		id,
	).Scan(
		&callback.ID,
		&callback.CallID,
		&callback.LeadID,
		&callback.RequestedTimeText,
		&callback.ScheduledFor,
		&callback.Timezone,
		&callback.ResolutionConfidence,
		&callback.ResolutionSource,
		&callback.ResolvedFrom,
		&callback.Status,
		&callback.ReminderSent,
		&callback.CallbackActionID,
		&callback.FollowUpCallID,
		&callback.CreatedAt,
		&callback.UpdatedAt,
	)

	return callback, err
}

func (r *CallbackRepo) GetByActionID(
	ctx context.Context,
	actionID uuid.UUID,
) (models.ScheduledCallback, error) {
	var callback models.ScheduledCallback

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			resolution_confidence,
			resolution_source,
			resolved_from,
			status,
			reminder_sent,
			callback_action_id,
			follow_up_call_id,
			created_at,
			updated_at
		FROM scheduled_callbacks
		WHERE callback_action_id = $1
		ORDER BY created_at DESC
		LIMIT 1
		`,
		actionID,
	).Scan(
		&callback.ID,
		&callback.CallID,
		&callback.LeadID,
		&callback.RequestedTimeText,
		&callback.ScheduledFor,
		&callback.Timezone,
		&callback.ResolutionConfidence,
		&callback.ResolutionSource,
		&callback.ResolvedFrom,
		&callback.Status,
		&callback.ReminderSent,
		&callback.CallbackActionID,
		&callback.FollowUpCallID,
		&callback.CreatedAt,
		&callback.UpdatedAt,
	)

	return callback, err
}

func (r *CallbackRepo) ClaimAndCreateFollowUpCall(
	ctx context.Context,
) (ClaimedCallback, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return ClaimedCallback{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var callback models.ScheduledCallback

	err = tx.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			resolution_confidence,
			resolution_source,
			resolved_from,
			status,
			reminder_sent,
			callback_action_id,
			follow_up_call_id,
			created_at,
			updated_at
		FROM scheduled_callbacks
		WHERE status IN (
			'scheduled',
			'rescheduled'
		)
		  AND scheduled_for IS NOT NULL
		  AND scheduled_for <= now()
		  AND follow_up_call_id IS NULL
		ORDER BY scheduled_for ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
		`,
	).Scan(
		&callback.ID,
		&callback.CallID,
		&callback.LeadID,
		&callback.RequestedTimeText,
		&callback.ScheduledFor,
		&callback.Timezone,
		&callback.ResolutionConfidence,
		&callback.ResolutionSource,
		&callback.ResolvedFrom,
		&callback.Status,
		&callback.ReminderSent,
		&callback.CallbackActionID,
		&callback.FollowUpCallID,
		&callback.CreatedAt,
		&callback.UpdatedAt,
	)
	if err != nil {
		return ClaimedCallback{}, err
	}

	var call models.Call

	err = tx.QueryRow(
		ctx,
		`
		INSERT INTO calls (
			lead_id,
			campaign_id,
			campaign_version,
			parent_call_id,
			provider,
			direction,
			status,
			attempt_number,
			primary_language
		)
		SELECT
			lead_id,
			campaign_id,
			campaign_version,
			id,
			'twilio',
			'outbound',
			'queued',
			attempt_number + 1,
			primary_language
		FROM calls
		WHERE id = $1
		RETURNING
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
		`,
		callback.CallID,
	).Scan(
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
	if err != nil {
		return ClaimedCallback{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			follow_up_call_id = $2,
			updated_at = now()
		WHERE id = $1
		  AND follow_up_call_id IS NULL
		  AND status IN (
			'scheduled',
			'rescheduled'
		  )
		`,
		callback.ID,
		call.ID,
	)
	if err != nil {
		return ClaimedCallback{}, err
	}

	callback.FollowUpCallID = &call.ID

	if err := tx.Commit(ctx); err != nil {
		return ClaimedCallback{}, err
	}

	return ClaimedCallback{
		Callback: callback,
		Call:     call,
	}, nil
}

func (r *CallbackRepo) MarkCallPlaced(
	ctx context.Context,
	callbackID uuid.UUID,
	callID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			follow_up_call_id = $2,
			updated_at = now()
		WHERE id = $1
		  AND status IN ('scheduled', 'rescheduled')
		  AND (
			follow_up_call_id IS NULL
			OR follow_up_call_id = $2
		  )
		`,
		callbackID,
		callID,
	)

	return err
}

func (r *CallbackRepo) RescheduleAfterFailure(
	ctx context.Context,
	callbackID uuid.UUID,
	delay time.Duration,
) error {
	if delay <= 0 {
		delay = 5 * time.Minute
	}

	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'rescheduled',
			scheduled_for = now() + $2::interval,
			follow_up_call_id = NULL,
			updated_at = now()
		WHERE id = $1
		  AND status IN ('scheduled', 'rescheduled')
		`,
		callbackID,
		delay.String(),
	)

	return err
}

func (r *CallbackRepo) MarkCompletedForFollowUpCall(
	ctx context.Context,
	callID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'completed',
			updated_at = now()
		WHERE follow_up_call_id = $1
		  AND status IN ('scheduled', 'rescheduled')
		`,
		callID,
	)

	return err
}

func (r *CallbackRepo) MarkRescheduledForFollowUpCall(
	ctx context.Context,
	callID uuid.UUID,
	delay time.Duration,
) error {
	if delay <= 0 {
		delay = 5 * time.Minute
	}

	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'rescheduled',
			scheduled_for = now() + $2::interval,
			follow_up_call_id = NULL,
			updated_at = now()
		WHERE follow_up_call_id = $1
		  AND status IN ('scheduled', 'rescheduled')
		`,
		callID,
		delay.String(),
	)

	return err
}

func (r *CallbackRepo) Reschedule(
	ctx context.Context,
	callbackID uuid.UUID,
	scheduledFor time.Time,
	timezone string,
) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var status models.CallbackStatus

	err = tx.QueryRow(
		ctx,
		`
		SELECT status
		FROM scheduled_callbacks
		WHERE id = $1
		FOR UPDATE
		`,
		callbackID,
	).Scan(&status)

	if err != nil {
		return err
	}

	switch status {
	case models.CallbackCompleted:
		return errors.New("callback is already completed")
	case models.CallbackCanceled:
		return errors.New("callback is canceled")
	case models.CallbackMissed:
		return errors.New("callback is already missed")
	}

	_, err = tx.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'rescheduled',
			scheduled_for = $2,
			timezone = $3,
			follow_up_call_id = NULL,
			updated_at = now()
		WHERE id = $1
		`,
		callbackID,
		scheduledFor,
		timezone,
	)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *CallbackRepo) MarkMissed(
	ctx context.Context,
	callbackID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'missed',
			follow_up_call_id = NULL,
			updated_at = now()
		WHERE id = $1
		  AND status IN ('scheduled', 'rescheduled')
		`,
		callbackID,
	)

	return err
}

func (r *CallbackRepo) MarkCompleted(
	ctx context.Context,
	callbackID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'completed',
			updated_at = now()
		WHERE id = $1
		  AND status NOT IN (
			'completed',
			'canceled',
			'missed'
		  )
		`,
		callbackID,
	)

	return err
}

func (r *CallbackRepo) MarkCanceled(
	ctx context.Context,
	callbackID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE scheduled_callbacks
		SET
			status = 'canceled',
			updated_at = now()
		WHERE id = $1
		  AND status NOT IN (
			'completed',
			'canceled',
			'missed'
		  )
		`,
		callbackID,
	)

	return err
}

func (r *CallbackRepo) Delete(
	ctx context.Context,
	callbackID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		DELETE FROM scheduled_callbacks
		WHERE id = $1
		`,
		callbackID,
	)

	return err
}

func (r *CallRepo) ActiveCampaignID(
	ctx context.Context,
) (uuid.UUID, error) {
	var campaignID uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT id
		FROM campaigns
		WHERE active = true
		  AND archived_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1
		`,
	).Scan(
		&campaignID,
	)

	return campaignID, err
}

func (r *CallbackRepo) List(
	ctx context.Context,
) ([]CallbackSummary, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			requested_time_text,
			scheduled_for,
			timezone,
			status,
			reminder_sent,
			created_at,
			updated_at
		FROM scheduled_callbacks
		ORDER BY
			CASE
				WHEN scheduled_for IS NULL THEN 1
				ELSE 0
			END,
			scheduled_for ASC,
			created_at ASC
		`,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	callbacks := make(
		[]CallbackSummary,
		0,
	)

	for rows.Next() {
		var callback CallbackSummary

		if err := rows.Scan(
			&callback.ID,
			&callback.CallID,
			&callback.LeadID,
			&callback.RequestedTimeText,
			&callback.ScheduledFor,
			&callback.Timezone,
			&callback.Status,
			&callback.ReminderSent,
			&callback.CreatedAt,
			&callback.UpdatedAt,
		); err != nil {
			return nil, err
		}

		callbacks = append(
			callbacks,
			callback,
		)
	}

	return callbacks, rows.Err()
}

func isNoRows(
	err error,
) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

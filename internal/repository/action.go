package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
	"elevate/internal/models"
)

type ActionRepo struct {
	db *database.DB
}

func NewActionRepo(
	db *database.DB,
) *ActionRepo {
	return &ActionRepo{
		db: db,
	}
}

type CreateActionInput struct {
	CallID           uuid.UUID
	ActionType       models.ActionType
	Trigger          models.ActionTrigger
	TriggerSegmentID *uuid.UUID
	Payload          map[string]any
	IdempotencyKey   string
	Priority         int16
	AvailableAt      *time.Time
}

const actionLeaseDuration = 2 * time.Minute

const actionColumns = `
	a.id,
	a.call_id,
	a.action_type,
	a.status,
	a.trigger,
	a.trigger_segment_id,
	a.payload,
	a.idempotency_key,
	a.available_at,
	a.attempt_count,
	a.max_attempts,
	a.locked_at,
	a.locked_by,
	a.lock_token,
	a.lock_expires_at,
	a.priority,
	a.last_error,
	a.triggered_at,
	a.completed_at,
	a.latency_ms,
	a.error_message
`

func scanAction(
	row interface {
		Scan(dest ...any) error
	},
	action *models.CallAction,
) error {
	return row.Scan(
		&action.ID,
		&action.CallID,
		&action.ActionType,
		&action.Status,
		&action.Trigger,
		&action.TriggerSegmentID,
		&action.Payload,
		&action.IdempotencyKey,
		&action.AvailableAt,
		&action.AttemptCount,
		&action.MaxAttempts,
		&action.LockedAt,
		&action.LockedBy,
		&action.LockToken,
		&action.LockExpiresAt,
		&action.Priority,
		&action.LastError,
		&action.TriggeredAt,
		&action.CompletedAt,
		&action.LatencyMs,
		&action.ErrorMessage,
	)
}

func (r *ActionRepo) Create(
	ctx context.Context,
	in CreateActionInput,
) (models.CallAction, error) {
	if r == nil ||
		r.db == nil {
		return models.CallAction{}, fmt.Errorf(
			"action: repository is not configured",
		)
	}

	payload, err := json.Marshal(
		in.Payload,
	)
	if err != nil {
		return models.CallAction{}, fmt.Errorf(
			"action: marshal payload: %w",
			err,
		)
	}

	idempotencyKey := strings.TrimSpace(
		in.IdempotencyKey,
	)

	var action models.CallAction

	err = r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO call_actions (
			call_id,
			action_type,
			status,
			trigger,
			trigger_segment_id,
			payload,
			idempotency_key,
			available_at,
			priority
		)
		VALUES (
			$1,
			$2,
			'pending'::action_status,
			$3,
			$4,
			$5,
			NULLIF($6, ''),
			COALESCE($7, now()),
			$8
		)
		ON CONFLICT (idempotency_key)
		WHERE idempotency_key IS NOT NULL
		DO UPDATE
		SET id = call_actions.id
		RETURNING
			id,
			call_id,
			action_type,
			status,
			trigger,
			trigger_segment_id,
			payload,
			idempotency_key,
			available_at,
			attempt_count,
			max_attempts,
			locked_at,
			locked_by,
			lock_token,
			lock_expires_at,
			priority,
			last_error,
			triggered_at,
			completed_at,
			latency_ms,
			error_message
		`,
		in.CallID,
		in.ActionType,
		in.Trigger,
		in.TriggerSegmentID,
		payload,
		idempotencyKey,
		in.AvailableAt,
		in.Priority,
	).Scan(
		&action.ID,
		&action.CallID,
		&action.ActionType,
		&action.Status,
		&action.Trigger,
		&action.TriggerSegmentID,
		&action.Payload,
		&action.IdempotencyKey,
		&action.AvailableAt,
		&action.AttemptCount,
		&action.MaxAttempts,
		&action.LockedAt,
		&action.LockedBy,
		&action.LockToken,
		&action.LockExpiresAt,
		&action.Priority,
		&action.LastError,
		&action.TriggeredAt,
		&action.CompletedAt,
		&action.LatencyMs,
		&action.ErrorMessage,
	)

	if err != nil {
		return models.CallAction{}, fmt.Errorf(
			"action: create: %w",
			err,
		)
	}

	return action, nil
}

func (r *ActionRepo) ClaimNext(
	ctx context.Context,
	workerID string,
) (models.CallAction, error) {
	if r == nil ||
		r.db == nil {
		return models.CallAction{}, fmt.Errorf(
			"action: repository is not configured",
		)
	}

	workerID = strings.TrimSpace(
		workerID,
	)

	if workerID == "" {
		return models.CallAction{}, fmt.Errorf(
			"action: worker ID is empty",
		)
	}

	lockToken := uuid.New()

	var action models.CallAction

	err := r.db.Pool.QueryRow(
		ctx,
		`
		WITH candidate AS (
			SELECT ca.id
			FROM call_actions AS ca
			WHERE ca.status = 'pending'::action_status
			  AND ca.available_at <= now()
			  AND ca.attempt_count < ca.max_attempts
			ORDER BY
				ca.priority ASC,
				ca.triggered_at ASC,
				ca.id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE call_actions AS a
		SET
			status = 'executing'::action_status,
			attempt_count = a.attempt_count + 1,
			locked_at = now(),
			locked_by = $1,
			lock_token = $2,
			lock_expires_at = now() + $3::interval,
			error_message = NULL
		FROM candidate AS c
		WHERE a.id = c.id
		RETURNING
			a.id,
			a.call_id,
			a.action_type,
			a.status,
			a.trigger,
			a.trigger_segment_id,
			a.payload,
			a.idempotency_key,
			a.available_at,
			a.attempt_count,
			a.max_attempts,
			a.locked_at,
			a.locked_by,
			a.lock_token,
			a.lock_expires_at,
			a.priority,
			a.last_error,
			a.triggered_at,
			a.completed_at,
			a.latency_ms,
			a.error_message
		`,
		workerID,
		lockToken,
		actionLeaseDuration.String(),
	).Scan(
		&action.ID,
		&action.CallID,
		&action.ActionType,
		&action.Status,
		&action.Trigger,
		&action.TriggerSegmentID,
		&action.Payload,
		&action.IdempotencyKey,
		&action.AvailableAt,
		&action.AttemptCount,
		&action.MaxAttempts,
		&action.LockedAt,
		&action.LockedBy,
		&action.LockToken,
		&action.LockExpiresAt,
		&action.Priority,
		&action.LastError,
		&action.TriggeredAt,
		&action.CompletedAt,
		&action.LatencyMs,
		&action.ErrorMessage,
	)

	if err != nil {
		return models.CallAction{}, err
	}

	return action, nil
}

func (r *ActionRepo) RefreshLease(
	ctx context.Context,
	actionID uuid.UUID,
	workerID string,
	lockToken *uuid.UUID,
) error {
	if r == nil ||
		r.db == nil {
		return fmt.Errorf(
			"action: repository is not configured",
		)
	}

	if actionID == uuid.Nil {
		return fmt.Errorf(
			"action: action ID is empty",
		)
	}

	if lockToken == nil ||
		*lockToken == uuid.Nil {
		return fmt.Errorf(
			"action: lock token is empty",
		)
	}

	workerID = strings.TrimSpace(
		workerID,
	)

	if workerID == "" {
		return fmt.Errorf(
			"action: worker ID is empty",
		)
	}

	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE call_actions
		SET
			lock_expires_at = now() + $4::interval
		WHERE id = $1
		  AND status = 'executing'::action_status
		  AND locked_by = $2
		  AND lock_token = $3
		  AND lock_expires_at IS NOT NULL
		  AND lock_expires_at > now()
		`,
		actionID,
		workerID,
		*lockToken,
		actionLeaseDuration.String(),
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (r *ActionRepo) RecoverExpired(
	ctx context.Context,
) error {
	if r == nil ||
		r.db == nil {
		return fmt.Errorf(
			"action: repository is not configured",
		)
	}

	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE call_actions
		SET
			status = CASE
				WHEN attempt_count >= max_attempts
				THEN 'failed'::action_status
				ELSE 'pending'::action_status
			END,

			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL,

			available_at = CASE
				WHEN attempt_count >= max_attempts
				THEN available_at
				ELSE now()
			END,

			error_message = CASE
				WHEN attempt_count >= max_attempts
				THEN COALESCE(
					error_message,
					'action lease expired after maximum attempts'
				)
				ELSE error_message
			END,

			last_error = CASE
				WHEN attempt_count >= max_attempts
				THEN COALESCE(
					last_error,
					jsonb_build_object(
						'message',
						'action lease expired after maximum attempts'
					)
				)
				ELSE last_error
			END,

			completed_at = CASE
				WHEN attempt_count >= max_attempts
				THEN COALESCE(
					completed_at,
					now()
				)
				ELSE completed_at
			END

		WHERE status = 'executing'::action_status
		  AND lock_expires_at IS NOT NULL
		  AND lock_expires_at < now()
		`,
	)

	return err
}

func (r *ActionRepo) MarkCompleted(
	ctx context.Context,
	actionID uuid.UUID,
	latencyMs *int,
) error {
	if r == nil ||
		r.db == nil {
		return fmt.Errorf(
			"action: repository is not configured",
		)
	}

	if actionID == uuid.Nil {
		return fmt.Errorf(
			"action: action ID is empty",
		)
	}

	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE call_actions
		SET
			status = 'completed'::action_status,
			completed_at = now(),
			latency_ms = COALESCE(
				$2,
				latency_ms
			),
			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL,
			error_message = NULL
		WHERE id = $1
		  AND status = 'executing'::action_status
		`,
		actionID,
		latencyMs,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (r *ActionRepo) MarkFailed(
	ctx context.Context,
	actionID uuid.UUID,
	message string,
) error {
	if r == nil ||
		r.db == nil {
		return fmt.Errorf(
			"action: repository is not configured",
		)
	}

	if actionID == uuid.Nil {
		return fmt.Errorf(
			"action: action ID is empty",
		)
	}

	message = strings.TrimSpace(
		message,
	)

	if message == "" {
		message = "action execution failed"
	}

	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE call_actions
		SET
			status = CASE
				WHEN attempt_count >= max_attempts
				THEN 'failed'::action_status
				ELSE 'pending'::action_status
			END,

			error_message = $2::text,

			last_error = jsonb_build_object(
				'message',
				$2::text,
				'failed_at',
				now()
			),

			available_at = CASE
				WHEN attempt_count >= max_attempts
				THEN available_at
				ELSE now() + interval '5 seconds'
			END,

			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL,

			completed_at = CASE
				WHEN attempt_count >= max_attempts
				THEN COALESCE(
					completed_at,
					now()
				)
				ELSE completed_at
			END

		WHERE id = $1
		  AND status = 'executing'::action_status
		`,
		actionID,
		message,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (r *ActionRepo) MarkSkipped(
	ctx context.Context,
	actionID uuid.UUID,
	reason string,
) error {
	if r == nil ||
		r.db == nil {
		return fmt.Errorf(
			"action: repository is not configured",
		)
	}

	if actionID == uuid.Nil {
		return fmt.Errorf(
			"action: action ID is empty",
		)
	}

	reason = strings.TrimSpace(
		reason,
	)

	if reason == "" {
		reason = "action skipped"
	}

	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE call_actions
		SET
			status = 'skipped'::action_status,
			completed_at = now(),
			error_message = $2::text,
			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL
		WHERE id = $1
		  AND status NOT IN (
			'completed'::action_status,
			'failed'::action_status,
			'skipped'::action_status
		  )
		`,
		actionID,
		reason,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

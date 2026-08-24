package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
)

type WebhookRepo struct {
	db *database.DB
}

func NewWebhookRepo(
	db *database.DB,
) *WebhookRepo {
	return &WebhookRepo{
		db: db,
	}
}

type WebhookEvent struct {
	ID              uuid.UUID
	Provider        string
	ProviderEventID *string
	PayloadHash     *string
	EventType       string
	Payload         []byte
	SignatureValid  *bool
	RelatedCallID   *uuid.UUID
	ReceivedAt      time.Time
	ProcessedAt     *time.Time
	Status          string
	ErrorMessage    *string
	AttemptCount    int
	MaxAttempts     int
	NextRetryAt     *time.Time
	LockedAt        *time.Time
	LockedBy        *string
	LockToken       *uuid.UUID
	LockExpiresAt   *time.Time
}

type CreateWebhookEventInput struct {
	Provider        string
	ProviderEventID *string
	EventType       string
	Payload         any
	SignatureValid  *bool
	RelatedCallID   *uuid.UUID
}

func NewWebhookPayloadHash(
	payload []byte,
) string {
	sum := sha256.Sum256(
		payload,
	)

	return hex.EncodeToString(
		sum[:],
	)
}

func (r *WebhookRepo) CreateOrGet(
	ctx context.Context,
	in CreateWebhookEventInput,
) (WebhookEvent, bool, error) {
	rawPayload, err := json.Marshal(
		in.Payload,
	)
	if err != nil {
		return WebhookEvent{}, false, fmt.Errorf(
			"webhook: marshal payload: %w",
			err,
		)
	}

	payloadHash := NewWebhookPayloadHash(
		rawPayload,
	)

	var event WebhookEvent

	err = r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO webhook_events (
			provider,
			provider_event_id,
			payload_hash,
			event_type,
			payload,
			signature_valid,
			related_call_id
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
		ON CONFLICT DO NOTHING
		RETURNING
			id,
			provider,
			provider_event_id,
			payload_hash,
			event_type,
			payload,
			signature_valid,
			related_call_id,
			received_at,
			processed_at,
			status,
			error_message,
			attempt_count,
			max_attempts,
			next_retry_at,
			locked_at,
			locked_by,
			lock_token,
			lock_expires_at
		`,
		in.Provider,
		in.ProviderEventID,
		payloadHash,
		in.EventType,
		rawPayload,
		in.SignatureValid,
		in.RelatedCallID,
	).Scan(
		&event.ID,
		&event.Provider,
		&event.ProviderEventID,
		&event.PayloadHash,
		&event.EventType,
		&event.Payload,
		&event.SignatureValid,
		&event.RelatedCallID,
		&event.ReceivedAt,
		&event.ProcessedAt,
		&event.Status,
		&event.ErrorMessage,
		&event.AttemptCount,
		&event.MaxAttempts,
		&event.NextRetryAt,
		&event.LockedAt,
		&event.LockedBy,
		&event.LockToken,
		&event.LockExpiresAt,
	)

	if err == nil {
		return event, true, nil
	}

	if err != pgx.ErrNoRows {
		return WebhookEvent{}, false, err
	}

	event, err = r.getExisting(
		ctx,
		in.Provider,
		in.ProviderEventID,
		payloadHash,
	)

	if err != nil {
		return WebhookEvent{}, false, err
	}

	return event, false, nil
}

func (r *WebhookRepo) getExisting(
	ctx context.Context,
	provider string,
	providerEventID *string,
	payloadHash string,
) (WebhookEvent, error) {
	var event WebhookEvent

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			provider,
			provider_event_id,
			payload_hash,
			event_type,
			payload,
			signature_valid,
			related_call_id,
			received_at,
			processed_at,
			status,
			error_message,
			attempt_count,
			max_attempts,
			next_retry_at,
			locked_at,
			locked_by,
			lock_token,
			lock_expires_at
		FROM webhook_events
		WHERE provider = $1
		  AND (
			(
				$2::text IS NOT NULL
				AND provider_event_id = $2
			)
			OR
			(
				$2::text IS NULL
				AND payload_hash = $3
			)
		  )
		ORDER BY received_at DESC
		LIMIT 1
		`,
		provider,
		providerEventID,
		payloadHash,
	).Scan(
		&event.ID,
		&event.Provider,
		&event.ProviderEventID,
		&event.PayloadHash,
		&event.EventType,
		&event.Payload,
		&event.SignatureValid,
		&event.RelatedCallID,
		&event.ReceivedAt,
		&event.ProcessedAt,
		&event.Status,
		&event.ErrorMessage,
		&event.AttemptCount,
		&event.MaxAttempts,
		&event.NextRetryAt,
		&event.LockedAt,
		&event.LockedBy,
		&event.LockToken,
		&event.LockExpiresAt,
	)

	return event, err
}

func (r *WebhookRepo) Claim(
	ctx context.Context,
	eventID uuid.UUID,
	workerID string,
) (bool, error) {
	lockToken := uuid.New()

	var claimedID uuid.UUID

	err := r.db.Pool.QueryRow(
		ctx,
		`
		UPDATE webhook_events
		SET
			status = 'processing',
			attempt_count = attempt_count + 1,
			locked_at = now(),
			locked_by = $2,
			lock_token = $3,
			lock_expires_at = now() + interval '2 minutes',
			error_message = NULL
		WHERE id = $1
		  AND attempt_count < max_attempts
		  AND (
			status = 'pending'
			OR (
				status = 'processing'
				AND lock_expires_at IS NOT NULL
				AND lock_expires_at < now()
			)
			OR (
				status = 'failed'
				AND (
					next_retry_at IS NULL
					OR next_retry_at <= now()
				)
			)
		  )
		RETURNING id
		`,
		eventID,
		workerID,
		lockToken,
	).Scan(
		&claimedID,
	)

	if err == pgx.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return claimedID == eventID, nil
}

func (r *WebhookRepo) MarkProcessed(
	ctx context.Context,
	eventID uuid.UUID,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE webhook_events
		SET
			status = 'processed',
			processed_at = now(),
			error_message = NULL,
			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL
		WHERE id = $1
		`,
		eventID,
	)

	return err
}

func (r *WebhookRepo) MarkFailed(
	ctx context.Context,
	eventID uuid.UUID,
	message string,
	retryAfter time.Duration,
) error {
	if retryAfter <= 0 {
		retryAfter = 5 * time.Second
	}

	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE webhook_events
		SET
			status = CASE
				WHEN attempt_count >= max_attempts
				THEN 'failed'
				ELSE 'pending'
			END,
			error_message = $2,
			next_retry_at = CASE
				WHEN attempt_count >= max_attempts
				THEN NULL
				ELSE now() + $3::interval
			END,
			locked_at = NULL,
			locked_by = NULL,
			lock_token = NULL,
			lock_expires_at = NULL
		WHERE id = $1
		`,
		eventID,
		message,
		retryAfter.String(),
	)

	return err
}

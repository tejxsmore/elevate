package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
	"elevate/internal/models"
)

type WhatsappRepo struct {
	db *database.DB
}

type CreateWhatsappInput struct {
	CallID         *uuid.UUID
	LeadID         uuid.UUID
	ActionID       *uuid.UUID
	MessageType    models.WhatsappMessageType
	ToNumber       string
	Body           string
	IdempotencyKey string
	SentDuringCall bool
}

func NewWhatsappRepo(db *database.DB) *WhatsappRepo {
	return &WhatsappRepo{db: db}
}

func (r *WhatsappRepo) Create(
	ctx context.Context,
	in CreateWhatsappInput,
) (models.WhatsappMessage, error) {
	var message models.WhatsappMessage

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO whatsapp_messages (
			call_id,
			lead_id,
			action_id,
			message_type,
			provider,
			to_number,
			body,
			idempotency_key,
			status,
			sent_during_call
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			'twilio',
			$5,
			$6,
			NULLIF($7, ''),
			'queued',
			$8
		)
		ON CONFLICT (idempotency_key)
		WHERE idempotency_key IS NOT NULL
		DO UPDATE SET
			id = whatsapp_messages.id
		RETURNING
			id,
			call_id,
			lead_id,
			action_id,
			message_type,
			provider,
			to_number,
			body,
			idempotency_key,
			provider_message_id,
			status,
			sent_during_call,
			sent_at,
			delivered_at,
			read_at,
			error_message,
			created_at
		`,
		in.CallID,
		in.LeadID,
		in.ActionID,
		in.MessageType,
		in.ToNumber,
		in.Body,
		in.IdempotencyKey,
		in.SentDuringCall,
	).Scan(
		&message.ID,
		&message.CallID,
		&message.LeadID,
		&message.ActionID,
		&message.MessageType,
		&message.Provider,
		&message.ToNumber,
		&message.Body,
		&message.IdempotencyKey,
		&message.ProviderMessageID,
		&message.Status,
		&message.SentDuringCall,
		&message.SentAt,
		&message.DeliveredAt,
		&message.ReadAt,
		&message.ErrorMessage,
		&message.CreatedAt,
	)

	return message, err
}

func (r *WhatsappRepo) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (models.WhatsappMessage, error) {
	var message models.WhatsappMessage

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			action_id,
			message_type,
			provider,
			to_number,
			body,
			idempotency_key,
			provider_message_id,
			status,
			sent_during_call,
			sent_at,
			delivered_at,
			read_at,
			error_message,
			created_at
		FROM whatsapp_messages
		WHERE id = $1
		`,
		id,
	).Scan(
		&message.ID,
		&message.CallID,
		&message.LeadID,
		&message.ActionID,
		&message.MessageType,
		&message.Provider,
		&message.ToNumber,
		&message.Body,
		&message.IdempotencyKey,
		&message.ProviderMessageID,
		&message.Status,
		&message.SentDuringCall,
		&message.SentAt,
		&message.DeliveredAt,
		&message.ReadAt,
		&message.ErrorMessage,
		&message.CreatedAt,
	)

	return message, err
}

func (r *WhatsappRepo) GetByProviderMessageID(
	ctx context.Context,
	providerMessageID string,
) (models.WhatsappMessage, error) {
	var message models.WhatsappMessage

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			call_id,
			lead_id,
			action_id,
			message_type,
			provider,
			to_number,
			body,
			idempotency_key,
			provider_message_id,
			status,
			sent_during_call,
			sent_at,
			delivered_at,
			read_at,
			error_message,
			created_at
		FROM whatsapp_messages
		WHERE provider_message_id = $1
		`,
		providerMessageID,
	).Scan(
		&message.ID,
		&message.CallID,
		&message.LeadID,
		&message.ActionID,
		&message.MessageType,
		&message.Provider,
		&message.ToNumber,
		&message.Body,
		&message.IdempotencyKey,
		&message.ProviderMessageID,
		&message.Status,
		&message.SentDuringCall,
		&message.SentAt,
		&message.DeliveredAt,
		&message.ReadAt,
		&message.ErrorMessage,
		&message.CreatedAt,
	)

	return message, err
}

func (r *WhatsappRepo) AttachAssets(
	ctx context.Context,
	messageID uuid.UUID,
	assetIDs []uuid.UUID,
) error {
	for _, assetID := range assetIDs {
		if assetID == uuid.Nil {
			continue
		}

		_, err := r.db.Pool.Exec(
			ctx,
			`
			INSERT INTO whatsapp_message_assets (
				whatsapp_message_id,
				asset_id
			)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
			`,
			messageID,
			assetID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *WhatsappRepo) Assets(
	ctx context.Context,
	messageID uuid.UUID,
) ([]models.WhatsappMessageAsset, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			whatsapp_message_id,
			asset_id
		FROM whatsapp_message_assets
		WHERE whatsapp_message_id = $1
		ORDER BY asset_id
		`,
		messageID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	assets := make(
		[]models.WhatsappMessageAsset,
		0,
	)

	for rows.Next() {
		var asset models.WhatsappMessageAsset

		if err := rows.Scan(
			&asset.WhatsappMessageID,
			&asset.AssetID,
		); err != nil {
			return nil, err
		}

		assets = append(
			assets,
			asset,
		)
	}

	return assets, rows.Err()
}

func (r *WhatsappRepo) MarkSent(
	ctx context.Context,
	id uuid.UUID,
	providerMessageID string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = CASE
				WHEN status IN ('delivered', 'read')
				THEN status
				ELSE 'sent'
			END,
			provider_message_id = COALESCE(
				provider_message_id,
				$2
			),
			sent_at = COALESCE(
				sent_at,
				now()
			),
			error_message = NULL
		WHERE id = $1
		`,
		id,
		providerMessageID,
	)

	return err
}

func (r *WhatsappRepo) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	message string,
) error {
	_, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = 'failed',
			error_message = $2
		WHERE id = $1
		`,
		id,
		message,
	)

	return err
}

func (r *WhatsappRepo) MarkDelivered(
	ctx context.Context,
	providerMessageID string,
) error {
	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = CASE
				WHEN status = 'read'
				THEN status
				ELSE 'delivered'
			END,
			delivered_at = COALESCE(
				delivered_at,
				now()
			),
			error_message = NULL
		WHERE provider_message_id = $1
		`,
		providerMessageID,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (r *WhatsappRepo) MarkRead(
	ctx context.Context,
	providerMessageID string,
) error {
	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = 'read',
			read_at = COALESCE(
				read_at,
				now()
			),
			delivered_at = COALESCE(
				delivered_at,
				now()
			),
			sent_at = COALESCE(
				sent_at,
				now()
			),
			error_message = NULL
		WHERE provider_message_id = $1
		`,
		providerMessageID,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (r *WhatsappRepo) MarkFailedByProviderMessageID(
	ctx context.Context,
	providerMessageID string,
	message string,
) error {
	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = 'failed',
			error_message = $2
		WHERE provider_message_id = $1
		`,
		providerMessageID,
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

func (r *WhatsappRepo) MarkUndelivered(
	ctx context.Context,
	providerMessageID string,
	message string,
) error {
	result, err := r.db.Pool.Exec(
		ctx,
		`
		UPDATE whatsapp_messages
		SET
			status = 'undelivered',
			error_message = $2
		WHERE provider_message_id = $1
		`,
		providerMessageID,
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

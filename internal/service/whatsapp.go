package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type WhatsappService struct {
	twilio   *TwilioClient
	messages *repository.WhatsappRepo
}

type WhatsappSendInput struct {
	CallID         *uuid.UUID
	LeadID         uuid.UUID
	ActionID       *uuid.UUID
	MessageType    models.WhatsappMessageType
	ToNumber       string
	Body           string
	MediaURLs      []string
	AssetIDs       []uuid.UUID
	IdempotencyKey string
	SentDuringCall bool
}

func NewWhatsappService(
	twilio *TwilioClient,
	messages *repository.WhatsappRepo,
) *WhatsappService {
	return &WhatsappService{
		twilio:   twilio,
		messages: messages,
	}
}

func (s *WhatsappService) Send(
	ctx context.Context,
	in WhatsappSendInput,
) (models.WhatsappMessage, error) {
	if s == nil ||
		s.twilio == nil ||
		s.messages == nil {
		return models.WhatsappMessage{}, fmt.Errorf(
			"whatsapp service is not configured",
		)
	}

	if strings.TrimSpace(
		in.ToNumber,
	) == "" {
		return models.WhatsappMessage{}, fmt.Errorf(
			"whatsapp: recipient is empty",
		)
	}

	in.ToNumber = normalizeWhatsappNumber(
		in.ToNumber,
	)

	in.Body = strings.TrimSpace(
		in.Body,
	)

	in.MediaURLs = normalizeMediaURLs(
		in.MediaURLs,
	)

	if in.Body == "" &&
		len(in.MediaURLs) == 0 {
		return models.WhatsappMessage{}, fmt.Errorf(
			"whatsapp: message body and media are both empty",
		)
	}

	if strings.TrimSpace(
		in.IdempotencyKey,
	) == "" {
		in.IdempotencyKey =
			BuildWhatsappIdempotencyKey(
				in.CallID,
				in.LeadID,
				in.MessageType,
				in.Body,
				in.MediaURLs,
			)
	}

	message, err := s.messages.Create(
		ctx,
		repository.CreateWhatsappInput{
			CallID:         in.CallID,
			LeadID:         in.LeadID,
			ActionID:       in.ActionID,
			MessageType:    in.MessageType,
			ToNumber:       in.ToNumber,
			Body:           in.Body,
			IdempotencyKey: in.IdempotencyKey,
			SentDuringCall: in.SentDuringCall,
		},
	)

	if err != nil {
		return models.WhatsappMessage{}, fmt.Errorf(
			"create whatsapp message: %w",
			err,
		)
	}

	if err := s.messages.AttachAssets(
		ctx,
		message.ID,
		in.AssetIDs,
	); err != nil {
		return models.WhatsappMessage{}, fmt.Errorf(
			"attach whatsapp assets: %w",
			err,
		)
	}

	switch message.Status {
	case models.WAStatusSent,
		models.WAStatusDelivered,
		models.WAStatusRead:
		return message, nil
	}

	if message.ProviderMessageID != nil {
		providerMessageID := strings.TrimSpace(
			*message.ProviderMessageID,
		)

		if providerMessageID != "" {
			return message, nil
		}
	}

	providerID, err := s.twilio.SendWhatsApp(
		ctx,
		in.ToNumber,
		in.Body,
		in.MediaURLs,
	)

	if err != nil {
		_ = s.messages.MarkFailed(
			ctx,
			message.ID,
			err.Error(),
		)

		return models.WhatsappMessage{}, fmt.Errorf(
			"send WhatsApp through Twilio: %w",
			err,
		)
	}

	providerID = strings.TrimSpace(
		providerID,
	)

	if providerID == "" {
		err := fmt.Errorf(
			"twilio returned empty WhatsApp message SID",
		)

		_ = s.messages.MarkFailed(
			ctx,
			message.ID,
			err.Error(),
		)

		return models.WhatsappMessage{}, err
	}

	if err := s.messages.MarkSent(
		ctx,
		message.ID,
		providerID,
	); err != nil {
		return models.WhatsappMessage{}, fmt.Errorf(
			"mark whatsapp sent: %w",
			err,
		)
	}

	updated, err := s.messages.GetByID(
		ctx,
		message.ID,
	)
	if err != nil {
		return models.WhatsappMessage{}, fmt.Errorf(
			"reload whatsapp message: %w",
			err,
		)
	}

	return updated, nil
}

func (s *WhatsappService) HandleStatus(
	ctx context.Context,
	providerMessageID string,
	status string,
	errorMessage string,
	eventType string,
) error {
	if s == nil ||
		s.messages == nil {
		return fmt.Errorf(
			"whatsapp service is not configured",
		)
	}

	providerMessageID = strings.TrimSpace(
		providerMessageID,
	)

	if providerMessageID == "" {
		return fmt.Errorf(
			"whatsapp status: missing provider message ID",
		)
	}

	status = strings.ToLower(
		strings.TrimSpace(status),
	)

	switch status {
	case "queued",
		"accepted",
		"sending":
		return nil

	case "sent":
		message, err :=
			s.messages.GetByProviderMessageID(
				ctx,
				providerMessageID,
			)
		if err != nil {
			return err
		}

		return s.messages.MarkSent(
			ctx,
			message.ID,
			providerMessageID,
		)

	case "delivered":
		return s.messages.MarkDelivered(
			ctx,
			providerMessageID,
		)

	case "read":
		return s.messages.MarkRead(
			ctx,
			providerMessageID,
		)

	case "failed":
		return s.messages.MarkFailedByProviderMessageID(
			ctx,
			providerMessageID,
			firstNonEmpty(
				errorMessage,
				"WhatsApp message delivery failed",
			),
		)

	case "undelivered":
		return s.messages.MarkUndelivered(
			ctx,
			providerMessageID,
			firstNonEmpty(
				errorMessage,
				"WhatsApp message was undelivered",
			),
		)

	default:
		return fmt.Errorf(
			"whatsapp status: unsupported status %q event=%q",
			status,
			eventType,
		)
	}
}

func BuildWhatsappIdempotencyKey(
	callID *uuid.UUID,
	leadID uuid.UUID,
	messageType models.WhatsappMessageType,
	body string,
	mediaURLs []string,
) string {
	callValue := ""

	if callID != nil {
		callValue = callID.String()
	}

	canonical := strings.Join(
		[]string{
			callValue,
			leadID.String(),
			string(messageType),
			strings.TrimSpace(body),
			strings.Join(
				normalizeMediaURLs(mediaURLs),
				"|",
			),
		},
		"|",
	)

	sum := sha256.Sum256(
		[]byte(canonical),
	)

	return "wa:" + hex.EncodeToString(
		sum[:16],
	)
}

func normalizeWhatsappNumber(
	value string,
) string {
	value = strings.TrimSpace(
		value,
	)

	if strings.HasPrefix(
		value,
		"whatsapp:",
	) {
		return value
	}

	return "whatsapp:" + value
}

func firstNonEmpty(
	values ...string,
) string {
	for _, value := range values {
		value = strings.TrimSpace(
			value,
		)

		if value != "" {
			return value
		}
	}

	return ""
}

func normalizeMediaURLs(
	values []string,
) []string {
	result := make(
		[]string,
		0,
		len(values),
	)

	seen := make(
		map[string]struct{},
	)

	for _, value := range values {
		value = strings.TrimSpace(
			value,
		)

		if value == "" {
			continue
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}

		result = append(
			result,
			value,
		)
	}

	return result
}

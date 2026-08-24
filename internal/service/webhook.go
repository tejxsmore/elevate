package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"elevate/internal/repository"
)

type WebhookService struct {
	repo *repository.WebhookRepo
}

func NewWebhookService(
	webhookRepo *repository.WebhookRepo,
) *WebhookService {
	return &WebhookService{
		repo: webhookRepo,
	}
}

type WebhookIngestResult struct {
	EventID     uuid.UUID
	ShouldRun   bool
	AlreadyDone bool
}

func (s *WebhookService) Ingest(
	ctx context.Context,
	provider string,
	eventType string,
	providerEventID string,
	payload any,
	relatedCallID *uuid.UUID,
) (WebhookIngestResult, error) {
	if s == nil ||
		s.repo == nil {
		return WebhookIngestResult{}, fmt.Errorf(
			"webhook service is not configured",
		)
	}

	provider = strings.TrimSpace(
		provider,
	)

	eventType = strings.TrimSpace(
		eventType,
	)

	providerEventID = strings.TrimSpace(
		providerEventID,
	)

	if provider == "" {
		return WebhookIngestResult{}, fmt.Errorf(
			"webhook provider is required",
		)
	}

	if eventType == "" {
		return WebhookIngestResult{}, fmt.Errorf(
			"webhook event type is required",
		)
	}

	var providerID *string

	if providerEventID != "" {
		providerID = &providerEventID
	}

	event, created, err :=
		s.repo.CreateOrGet(
			ctx,
			repository.CreateWebhookEventInput{
				Provider:        provider,
				ProviderEventID: providerID,
				EventType:       eventType,
				Payload:         payload,
				RelatedCallID:   relatedCallID,
			},
		)

	if err != nil {
		return WebhookIngestResult{}, fmt.Errorf(
			"webhook: create or get event: %w",
			err,
		)
	}

	if !created {
		switch strings.ToLower(
			strings.TrimSpace(event.Status),
		) {
		case "processed":
			return WebhookIngestResult{
				EventID:     event.ID,
				ShouldRun:   false,
				AlreadyDone: true,
			}, nil

		case "processing":
			if event.LockExpiresAt != nil &&
				event.LockExpiresAt.After(
					time.Now(),
				) {
				return WebhookIngestResult{
					EventID:   event.ID,
					ShouldRun: false,
				}, nil
			}
		}

		if event.Status == "failed" &&
			event.AttemptCount >= event.MaxAttempts {
			return WebhookIngestResult{
				EventID:     event.ID,
				ShouldRun:   false,
				AlreadyDone: true,
			}, nil
		}
	}

	workerID := fmt.Sprintf(
		"webhook-api-%s",
		uuid.NewString(),
	)

	claimed, err := s.repo.Claim(
		ctx,
		event.ID,
		workerID,
	)

	if err != nil {
		return WebhookIngestResult{}, fmt.Errorf(
			"webhook: claim event: %w",
			err,
		)
	}

	return WebhookIngestResult{
		EventID:   event.ID,
		ShouldRun: claimed,
	}, nil
}

func (s *WebhookService) Processed(
	ctx context.Context,
	eventID uuid.UUID,
) error {
	if s == nil ||
		s.repo == nil {
		return fmt.Errorf(
			"webhook service is not configured",
		)
	}

	if eventID == uuid.Nil {
		return fmt.Errorf(
			"webhook event ID is empty",
		)
	}

	return s.repo.MarkProcessed(
		ctx,
		eventID,
	)
}

func (s *WebhookService) Failed(
	ctx context.Context,
	eventID uuid.UUID,
	err error,
) error {
	if err == nil {
		return nil
	}

	if s == nil ||
		s.repo == nil {
		return fmt.Errorf(
			"webhook service is not configured",
		)
	}

	if eventID == uuid.Nil {
		return fmt.Errorf(
			"webhook event ID is empty",
		)
	}

	return s.repo.MarkFailed(
		ctx,
		eventID,
		err.Error(),
		5*time.Second,
	)
}

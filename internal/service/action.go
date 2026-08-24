package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type ActionService struct {
	repo *repository.ActionRepo
}

func NewActionService(
	r *repository.ActionRepo,
) *ActionService {
	return &ActionService{
		repo: r,
	}
}

func (s *ActionService) Ensure(
	ctx context.Context,
	callID uuid.UUID,
	actionType models.ActionType,
	trigger models.ActionTrigger,
	triggerSegmentID *uuid.UUID,
	payload map[string]any,
	idempotencyKey string,
) (models.CallAction, error) {
	if s == nil || s.repo == nil {
		return models.CallAction{}, fmt.Errorf(
			"action service is not configured",
		)
	}

	if strings.TrimSpace(
		idempotencyKey,
	) == "" {
		idempotencyKey =
			BuildActionIdempotencyKey(
				callID,
				actionType,
				payload,
			)
	}

	return s.repo.Create(
		ctx,
		repository.CreateActionInput{
			CallID:           callID,
			ActionType:       actionType,
			Trigger:          trigger,
			TriggerSegmentID: triggerSegmentID,
			Payload:          payload,
			IdempotencyKey:   idempotencyKey,
			Priority:         actionPriority(actionType),
		},
	)
}

func BuildActionIdempotencyKey(
	callID uuid.UUID,
	actionType models.ActionType,
	payload map[string]any,
) string {
	canonical, err := json.Marshal(payload)

	if err != nil {
		canonical = []byte(
			fmt.Sprintf("%v", payload),
		)
	}

	sum := sha256.Sum256(
		canonical,
	)

	return fmt.Sprintf(
		"%s:%s:%s",
		callID.String(),
		actionType,
		hex.EncodeToString(sum[:12]),
	)
}

func actionPriority(
	actionType models.ActionType,
) int16 {
	switch actionType {
	case models.ActionWhatsappMidCall:
		return 1

	case models.ActionScheduleCallback:
		return 2

	case models.ActionWhatsappBrochure:
		return 5

	case models.ActionWhatsappResume:
		return 5

	case models.ActionWhatsappFollowup:
		return 10

	case models.ActionUpdateClassification:
		return 20

	default:
		return 50
	}
}

func (s *ActionService) Complete(
	ctx context.Context,
	actionID uuid.UUID,
	start time.Time,
) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf(
			"action service is not configured",
		)
	}

	latency := int(
		time.Since(start).Milliseconds(),
	)

	return s.repo.MarkCompleted(
		ctx,
		actionID,
		&latency,
	)
}

func (s *ActionService) Fail(
	ctx context.Context,
	actionID uuid.UUID,
	err error,
) error {
	if err == nil {
		return nil
	}

	if s == nil || s.repo == nil {
		return fmt.Errorf(
			"action service is not configured",
		)
	}

	return s.repo.MarkFailed(
		ctx,
		actionID,
		err.Error(),
	)
}

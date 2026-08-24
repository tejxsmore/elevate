package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type DecisionEngine struct {
	cfg                *config.Config
	discoveryRepo      *repository.DiscoveryRepo
	classificationRepo *repository.ClassificationRepo
	actionService      *ActionService
	callbackService    *CallbackService
	callRepo           *repository.CallRepo
}

func NewDecisionEngine(
	cfg *config.Config,
	discoveryRepo *repository.DiscoveryRepo,
	classificationRepo *repository.ClassificationRepo,
	actionService *ActionService,
	callbackService *CallbackService,
	callRepo *repository.CallRepo,
) *DecisionEngine {
	return &DecisionEngine{
		cfg:                cfg,
		discoveryRepo:      discoveryRepo,
		classificationRepo: classificationRepo,
		actionService:      actionService,
		callbackService:    callbackService,
		callRepo:           callRepo,
	}
}

func (d *DecisionEngine) ProcessUserText(
	ctx context.Context,
	callID uuid.UUID,
	content string,
	segmentID *uuid.UUID,
) error {
	if d == nil {
		return fmt.Errorf(
			"decision engine is not configured",
		)
	}

	content = strings.TrimSpace(content)

	if content == "" {
		return nil
	}

	if d.discoveryRepo == nil {
		return fmt.Errorf(
			"decision: discovery repository is not configured",
		)
	}

	if d.classificationRepo == nil {
		return fmt.Errorf(
			"decision: classification repository is not configured",
		)
	}

	if d.actionService == nil {
		return fmt.Errorf(
			"decision: action service is not configured",
		)
	}

	if d.callRepo == nil {
		return fmt.Errorf(
			"decision: call repository is not configured",
		)
	}

	extractor := NewDiscoveryExtractor()

	extracted := extractor.Extract(
		content,
	)

	var update repository.DiscoveryUpdate

	if extracted.BusinessNiche != "" {
		update.BusinessNiche =
			&extracted.BusinessNiche
	}

	if extracted.ProductsSold != "" {
		update.ProductsSold =
			&extracted.ProductsSold
	}

	if extracted.ProductCountEstimate != "" {
		update.ProductCountEstimate =
			&extracted.ProductCountEstimate
	}

	if extracted.BudgetRange != "" {
		update.BudgetRange =
			&extracted.BudgetRange
	}

	if extracted.BudgetRawText != "" {
		update.BudgetRawText =
			&extracted.BudgetRawText
	}

	if extracted.Timeline != "" {
		update.Timeline =
			&extracted.Timeline
	}

	if extracted.TimelineRawText != "" {
		update.TimelineRawText =
			&extracted.TimelineRawText
	}

	update.FeaturesRequested =
		extracted.FeaturesRequested

	if extracted.HasBarrier &&
		strings.TrimSpace(
			extracted.BarrierDetail,
		) != "" {
		update.ExtraNotes =
			&extracted.BarrierDetail
	}

	profile, err := d.discoveryRepo.Upsert(
		ctx,
		callID,
		update,
	)
	if err != nil {
		return fmt.Errorf(
			"decision: update discovery: %w",
			err,
		)
	}

	if extracted.HasBarrier {
		if err := d.discoveryRepo.AddBarrier(
			ctx,
			callID,
			extracted.BarrierType,
			extracted.BarrierDetail,
			content,
		); err != nil {
			return fmt.Errorf(
				"decision: store barrier: %w",
				err,
			)
		}
	}

	result := NewClassifier().Classify(
		content,
		profile,
		extracted.HasBarrier,
		extracted.BarrierType,
	)

	/*
		The PostgreSQL trigger
		trg_sync_classification automatically
		synchronizes the inserted classification
		into the calls table.

		Therefore we only insert the classification here.
	*/
	_, err = d.classificationRepo.Create(
		ctx,
		callID,
		result.Label,
		result.Confidence,
		result.Summary,
		result.Signals,
		segmentID,
	)
	if err != nil {
		return fmt.Errorf(
			"decision: persist classification: %w",
			err,
		)
	}

	if extracted.CallbackRequest != "" {
		if err := d.handleCallbackRequest(
			ctx,
			callID,
			extracted.CallbackRequest,
			content,
		); err != nil {
			return err
		}
	}

	switch result.Label {
	case models.ClassificationHot:
		return d.createHotAction(
			ctx,
			callID,
			segmentID,
			content,
			profile,
		)

	case models.ClassificationWarm:
		return d.createWarmAction(
			ctx,
			callID,
			segmentID,
			content,
			profile,
		)

	case models.ClassificationCold:
		return d.createColdAction(
			ctx,
			callID,
			segmentID,
			content,
		)

	default:
		return nil
	}
}

func (d *DecisionEngine) createHotAction(
	ctx context.Context,
	callID uuid.UUID,
	segmentID *uuid.UUID,
	quote string,
	profile models.DiscoveryProfile,
) error {
	_, err := d.actionService.Ensure(
		ctx,
		callID,
		models.ActionWhatsappMidCall,
		models.TriggerIntentDetected,
		segmentID,
		map[string]any{
			"classification": "hot",
			"quote":          quote,
			"business_niche": profile.BusinessNiche,
			"products":       profile.ProductsSold,
			"budget":         profile.BudgetRange,
			"timeline":       profile.Timeline,
			"features":       profile.FeaturesRequested,
		},
		fmt.Sprintf(
			"%s:%s",
			callID,
			models.ActionWhatsappMidCall,
		),
	)

	if err != nil {
		return fmt.Errorf(
			"decision: create hot action: %w",
			err,
		)
	}

	return nil
}

func (d *DecisionEngine) createWarmAction(
	ctx context.Context,
	callID uuid.UUID,
	segmentID *uuid.UUID,
	quote string,
	profile models.DiscoveryProfile,
) error {
	_, err := d.actionService.Ensure(
		ctx,
		callID,
		models.ActionUpdateClassification,
		models.TriggerIntentDetected,
		segmentID,
		map[string]any{
			"classification": "warm",
			"quote":          quote,
			"budget":         profile.BudgetRange,
			"timeline":       profile.Timeline,
		},
		fmt.Sprintf(
			"%s:%s",
			callID,
			models.ActionUpdateClassification,
		),
	)

	if err != nil {
		return fmt.Errorf(
			"decision: create warm action: %w",
			err,
		)
	}

	return nil
}

func (d *DecisionEngine) createColdAction(
	ctx context.Context,
	callID uuid.UUID,
	segmentID *uuid.UUID,
	quote string,
) error {
	_, err := d.actionService.Ensure(
		ctx,
		callID,
		models.ActionWhatsappBrochure,
		models.TriggerIntentDetected,
		segmentID,
		map[string]any{
			"classification": "cold",
			"quote":          quote,
		},
		fmt.Sprintf(
			"%s:%s",
			callID,
			models.ActionWhatsappBrochure,
		),
	)

	if err != nil {
		return fmt.Errorf(
			"decision: create cold action: %w",
			err,
		)
	}

	return nil
}

func (d *DecisionEngine) handleCallbackRequest(
	ctx context.Context,
	callID uuid.UUID,
	request string,
	quote string,
) error {
	request = strings.TrimSpace(
		request,
	)

	if request == "" {
		return nil
	}

	_, err := d.actionService.Ensure(
		ctx,
		callID,
		models.ActionScheduleCallback,
		models.TriggerUserRequestedTime,
		nil,
		map[string]any{
			"requested_time": request,
			"quote":          quote,
			"timezone":       "Asia/Kolkata",
		},
		fmt.Sprintf(
			"%s:%s:%s",
			callID,
			models.ActionScheduleCallback,
			request,
		),
	)

	if err != nil {
		return fmt.Errorf(
			"decision: create callback action: %w",
			err,
		)
	}

	return nil
}

func (d *DecisionEngine) FinishCall(
	ctx context.Context,
	callID uuid.UUID,
) error {
	if d == nil {
		return fmt.Errorf(
			"decision engine is not configured",
		)
	}

	if d.callRepo == nil {
		return fmt.Errorf(
			"decision: call repository is not configured",
		)
	}

	if d.actionService == nil {
		return fmt.Errorf(
			"decision: action service is not configured",
		)
	}

	call, err := d.callRepo.Get(
		ctx,
		callID,
	)
	if err != nil {
		return fmt.Errorf(
			"decision: get call: %w",
			err,
		)
	}

	_, err = d.actionService.Ensure(
		ctx,
		callID,
		models.ActionWhatsappFollowup,
		models.TriggerCallEnded,
		nil,
		map[string]any{
			"classification": call.CurrentClassification,
		},
		fmt.Sprintf(
			"%s:%s",
			callID,
			models.ActionWhatsappFollowup,
		),
	)

	if err != nil {
		return fmt.Errorf(
			"decision: create follow-up action: %w",
			err,
		)
	}

	return nil
}

func nowInIndiaTime() time.Time {
	location, err := time.LoadLocation(
		"Asia/Kolkata",
	)

	if err != nil {
		return time.Now()
	}

	return time.Now().In(location)
}

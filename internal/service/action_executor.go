package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type ActionExecutor struct {
	cfg            *config.Config
	whatsapp       *WhatsappService
	callbacks      *CallbackService
	callRepo       *repository.CallRepo
	leadRepo       *repository.LeadRepo
	campaignRepo   *repository.CampaignRepo
	assetService   *AssetService
	callbackRepo   *repository.CallbackRepo
	discoveryRepo  *repository.DiscoveryRepo
	classification *repository.ClassificationRepo
	convRepo       *repository.ConversationRepo
	messageBuilder *WhatsAppMessageBuilder
}

func NewActionExecutor(
	cfg *config.Config,
	whatsapp *WhatsappService,
	callbacks *CallbackService,
	callRepo *repository.CallRepo,
	leadRepo *repository.LeadRepo,
	campaignRepo *repository.CampaignRepo,
	assetService *AssetService,
	callbackRepo *repository.CallbackRepo,
	discoveryRepo *repository.DiscoveryRepo,
	convRepo *repository.ConversationRepo,
	classificationRepo *repository.ClassificationRepo,
) *ActionExecutor {
	return &ActionExecutor{
		cfg:            cfg,
		whatsapp:       whatsapp,
		callbacks:      callbacks,
		callRepo:       callRepo,
		leadRepo:       leadRepo,
		campaignRepo:   campaignRepo,
		assetService:   assetService,
		callbackRepo:   callbackRepo,
		discoveryRepo:  discoveryRepo,
		classification: classificationRepo,
		convRepo:       convRepo,
		messageBuilder: NewWhatsAppMessageBuilder(),
	}
}

func (e *ActionExecutor) Execute(
	ctx context.Context,
	action models.CallAction,
) error {
	if e == nil {
		return fmt.Errorf(
			"action executor is not configured",
		)
	}

	switch action.ActionType {
	case models.ActionWhatsappMidCall:
		return e.executeMidCallWhatsApp(
			ctx,
			action,
		)

	case models.ActionWhatsappBrochure:
		return e.executeBrochureWhatsApp(
			ctx,
			action,
		)

	case models.ActionWhatsappFollowup:
		return e.executeFollowupWhatsApp(
			ctx,
			action,
		)

	case models.ActionWhatsappResume:
		return e.executeResumeWhatsApp(
			ctx,
			action,
		)

	case models.ActionScheduleCallback:
		return e.executeScheduleCallback(
			ctx,
			action,
		)

	case models.ActionUpdateClassification:
		return nil

	default:
		return fmt.Errorf(
			"unsupported action type: %s",
			action.ActionType,
		)
	}
}

func (e *ActionExecutor) executeMidCallWhatsApp(
	ctx context.Context,
	action models.CallAction,
) error {
	if e.whatsapp == nil {
		return fmt.Errorf(
			"mid-call WhatsApp: WhatsApp service is not configured",
		)
	}

	call, err := e.callRepo.Get(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"mid-call WhatsApp: get call: %w",
			err,
		)
	}

	lead, err := e.leadRepo.Get(
		ctx,
		call.LeadID,
	)
	if err != nil {
		return fmt.Errorf(
			"mid-call WhatsApp: get lead: %w",
			err,
		)
	}

	discovery, err := e.discoveryRepo.GetByCallID(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"mid-call WhatsApp: get discovery: %w",
			err,
		)
	}

	payload := decodeActionPayload(
		action.Payload,
	)

	quote, _ := payload["quote"].(string)

	body := e.messageBuilder.BuildMidCall(
		lead,
		discovery,
		quote,
	)

	_, err = e.whatsapp.Send(
		ctx,
		WhatsappSendInput{
			CallID:      &action.CallID,
			LeadID:      call.LeadID,
			ActionID:    &action.ID,
			MessageType: models.WAMessageTypeMidCallIntent,
			ToNumber:    lead.PhoneE164,
			Body:        body,
			IdempotencyKey: resolveActionIdempotencyKey(
				action,
				payload,
			),
			SentDuringCall: true,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"mid-call WhatsApp: send: %w",
			err,
		)
	}

	return nil
}

func (e *ActionExecutor) executeBrochureWhatsApp(
	ctx context.Context,
	action models.CallAction,
) error {
	if e.whatsapp == nil {
		return fmt.Errorf(
			"brochure WhatsApp: WhatsApp service is not configured",
		)
	}

	call, err := e.callRepo.Get(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"brochure WhatsApp: get call: %w",
			err,
		)
	}

	lead, err := e.leadRepo.Get(
		ctx,
		call.LeadID,
	)
	if err != nil {
		return fmt.Errorf(
			"brochure WhatsApp: get lead: %w",
			err,
		)
	}

	discovery, err := e.discoveryRepo.GetByCallID(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"brochure WhatsApp: get discovery: %w",
			err,
		)
	}

	body := e.messageBuilder.BuildBrochure(
		lead,
		discovery,
	)

	assets, err := e.loadCampaignAssets(
		ctx,
		call.CampaignID,
		false,
		true,
	)
	if err != nil {
		return fmt.Errorf(
			"brochure WhatsApp: load asset: %w",
			err,
		)
	}

	payload := decodeActionPayload(
		action.Payload,
	)

	_, err = e.whatsapp.Send(
		ctx,
		WhatsappSendInput{
			CallID:      &action.CallID,
			LeadID:      call.LeadID,
			ActionID:    &action.ID,
			MessageType: models.WAMessageTypeBrochure,
			ToNumber:    lead.PhoneE164,
			Body:        body,
			MediaURLs:   assetURLs(assets),
			AssetIDs:    assetIDs(assets),
			IdempotencyKey: resolveActionIdempotencyKey(
				action,
				payload,
			),
			SentDuringCall: true,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"brochure WhatsApp: send: %w",
			err,
		)
	}

	return nil
}

func (e *ActionExecutor) executeFollowupWhatsApp(
	ctx context.Context,
	action models.CallAction,
) error {
	if e.whatsapp == nil {
		return fmt.Errorf(
			"follow-up WhatsApp: WhatsApp service is not configured",
		)
	}

	call, err := e.callRepo.Get(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: get call: %w",
			err,
		)
	}

	lead, err := e.leadRepo.Get(
		ctx,
		call.LeadID,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: get lead: %w",
			err,
		)
	}

	discovery, err := e.discoveryRepo.GetByCallID(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: get discovery: %w",
			err,
		)
	}

	campaign, err := e.loadCampaign(
		ctx,
		call.CampaignID,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: get campaign: %w",
			err,
		)
	}

	messages, err := e.convRepo.RecentMessages(
		ctx,
		action.CallID,
		20,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: get conversation: %w",
			err,
		)
	}

	classification :=
		models.ClassificationUnclassified

	if e.classification != nil {
		value, classificationErr :=
			e.classification.Latest(
				ctx,
				action.CallID,
			)

		if classificationErr != nil &&
			!errors.Is(
				classificationErr,
				pgx.ErrNoRows,
			) {
			return fmt.Errorf(
				"follow-up WhatsApp: get classification: %w",
				classificationErr,
			)
		}

		if classificationErr == nil {
			classification =
				value.Classification
		}
	}

	body := e.messageBuilder.BuildFollowup(
		WhatsAppFollowupContext{
			Lead:           lead,
			Call:           call,
			Discovery:      discovery,
			Messages:       messages,
			Classification: classification,
			Campaign:       campaign,
		},
	)

	assets, err := e.loadCampaignAssets(
		ctx,
		call.CampaignID,
		true,
		true,
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: load required assets: %w",
			err,
		)
	}

	if len(assets) < 2 {
		return fmt.Errorf(
			"follow-up WhatsApp: required resume and architecture assets are not both available",
		)
	}

	payload := decodeActionPayload(
		action.Payload,
	)

	_, err = e.whatsapp.Send(
		ctx,
		WhatsappSendInput{
			CallID:      &action.CallID,
			LeadID:      call.LeadID,
			ActionID:    &action.ID,
			MessageType: models.WAMessageTypePostCallFollowup,
			ToNumber:    lead.PhoneE164,
			Body:        body,
			MediaURLs:   assetURLs(assets),
			AssetIDs:    assetIDs(assets),
			IdempotencyKey: resolveActionIdempotencyKey(
				action,
				payload,
			),
			SentDuringCall: false,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"follow-up WhatsApp: send: %w",
			err,
		)
	}

	return nil
}

func (e *ActionExecutor) executeResumeWhatsApp(
	ctx context.Context,
	action models.CallAction,
) error {
	if e.whatsapp == nil {
		return fmt.Errorf(
			"resume WhatsApp: WhatsApp service is not configured",
		)
	}

	call, err := e.callRepo.Get(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"resume WhatsApp: get call: %w",
			err,
		)
	}

	lead, err := e.leadRepo.Get(
		ctx,
		call.LeadID,
	)
	if err != nil {
		return fmt.Errorf(
			"resume WhatsApp: get lead: %w",
			err,
		)
	}

	body := e.messageBuilder.BuildResume(
		lead,
	)

	assets, err := e.loadCampaignAssets(
		ctx,
		call.CampaignID,
		true,
		false,
	)
	if err != nil {
		return fmt.Errorf(
			"resume WhatsApp: load resume asset: %w",
			err,
		)
	}

	if len(assets) == 0 {
		return fmt.Errorf(
			"resume WhatsApp: resume asset is not available",
		)
	}

	payload := decodeActionPayload(
		action.Payload,
	)

	_, err = e.whatsapp.Send(
		ctx,
		WhatsappSendInput{
			CallID:      &action.CallID,
			LeadID:      call.LeadID,
			ActionID:    &action.ID,
			MessageType: models.WAMessageTypeResumeSend,
			ToNumber:    lead.PhoneE164,
			Body:        body,
			MediaURLs:   assetURLs(assets),
			AssetIDs:    assetIDs(assets),
			IdempotencyKey: resolveActionIdempotencyKey(
				action,
				payload,
			),
			SentDuringCall: false,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"resume WhatsApp: send: %w",
			err,
		)
	}

	return nil
}

func (e *ActionExecutor) executeScheduleCallback(
	ctx context.Context,
	action models.CallAction,
) error {
	if e.callbacks == nil {
		return fmt.Errorf(
			"callback service is not configured",
		)
	}

	if e.callbackRepo == nil {
		return fmt.Errorf(
			"callback repository is not configured",
		)
	}

	var payload struct {
		RequestedTime string `json:"requested_time"`
		Quote         string `json:"quote"`
		Timezone      string `json:"timezone"`
	}

	if len(action.Payload) == 0 {
		return fmt.Errorf(
			"callback action payload is empty",
		)
	}

	if err := json.Unmarshal(
		action.Payload,
		&payload,
	); err != nil {
		return fmt.Errorf(
			"callback action payload: %w",
			err,
		)
	}

	payload.RequestedTime =
		strings.TrimSpace(
			payload.RequestedTime,
		)

	if payload.RequestedTime == "" {
		return fmt.Errorf(
			"callback action missing requested_time",
		)
	}

	payload.Timezone =
		strings.TrimSpace(
			payload.Timezone,
		)

	if payload.Timezone == "" {
		payload.Timezone = "Asia/Kolkata"
	}

	existing, err :=
		e.callbackRepo.GetByActionID(
			ctx,
			action.ID,
		)

	if err == nil {
		switch existing.Status {
		case models.CallbackScheduled,
			models.CallbackNeedsConfirmation,
			models.CallbackCompleted,
			models.CallbackCanceled,
			models.CallbackMissed:
			return nil
		}
	}

	if err != nil &&
		!errors.Is(
			err,
			pgx.ErrNoRows,
		) {
		return fmt.Errorf(
			"callback action: lookup existing callback: %w",
			err,
		)
	}

	call, err := e.callRepo.Get(
		ctx,
		action.CallID,
	)
	if err != nil {
		return fmt.Errorf(
			"callback action: get call: %w",
			err,
		)
	}

	resolution := e.callbacks.Resolve(
		payload.RequestedTime,
		time.Now(),
		payload.Timezone,
	)

	if resolution.ResolvedFrom == nil {
		resolution.ResolvedFrom =
			map[string]any{}
	}

	resolution.ResolvedFrom["quote"] =
		strings.TrimSpace(
			payload.Quote,
		)

	_, err = e.callbacks.Create(
		ctx,
		call.ID,
		call.LeadID,
		payload.RequestedTime,
		resolution,
		&action.ID,
	)
	if err != nil {
		return fmt.Errorf(
			"callback action: create callback: %w",
			err,
		)
	}

	return nil
}

func (e *ActionExecutor) loadCampaign(
	ctx context.Context,
	campaignID *uuid.UUID,
) (models.Campaign, error) {
	if campaignID == nil {
		return models.Campaign{}, fmt.Errorf(
			"call has no campaign",
		)
	}

	if e.campaignRepo == nil {
		return models.Campaign{}, fmt.Errorf(
			"campaign repository is not configured",
		)
	}

	return e.campaignRepo.Get(
		ctx,
		*campaignID,
	)
}

func (e *ActionExecutor) loadCampaignAssets(
	ctx context.Context,
	campaignID *uuid.UUID,
	includeResume bool,
	includeDiagram bool,
) ([]AssetReference, error) {
	if campaignID == nil {
		return nil, fmt.Errorf(
			"call has no campaign",
		)
	}

	if e.assetService == nil {
		return nil, fmt.Errorf(
			"asset service is not configured",
		)
	}

	if e.campaignRepo == nil {
		return nil, fmt.Errorf(
			"campaign repository is not configured",
		)
	}

	campaign, err := e.campaignRepo.Get(
		ctx,
		*campaignID,
	)
	if err != nil {
		return nil, err
	}

	assets := make(
		[]AssetReference,
		0,
		2,
	)

	if includeResume {
		if campaign.DefaultResumeAssetID == nil {
			return nil, fmt.Errorf(
				"campaign %s has no default resume asset",
				campaign.ID,
			)
		}

		asset, err :=
			e.assetService.Reference(
				ctx,
				*campaign.DefaultResumeAssetID,
			)
		if err != nil {
			return nil, fmt.Errorf(
				"load resume asset: %w",
				err,
			)
		}

		assets = append(
			assets,
			asset,
		)
	}

	if includeDiagram {
		if campaign.DefaultDiagramAssetID == nil {
			return nil, fmt.Errorf(
				"campaign %s has no default architecture asset",
				campaign.ID,
			)
		}

		asset, err :=
			e.assetService.Reference(
				ctx,
				*campaign.DefaultDiagramAssetID,
			)
		if err != nil {
			return nil, fmt.Errorf(
				"load architecture asset: %w",
				err,
			)
		}

		assets = append(
			assets,
			asset,
		)
	}

	return assets, nil
}

func assetURLs(
	assets []AssetReference,
) []string {
	urls := make(
		[]string,
		0,
		len(assets),
	)

	for _, asset := range assets {
		value := strings.TrimSpace(
			asset.URL,
		)

		if value == "" {
			continue
		}

		urls = append(
			urls,
			value,
		)
	}

	return urls
}

func assetIDs(
	assets []AssetReference,
) []uuid.UUID {
	ids := make(
		[]uuid.UUID,
		0,
		len(assets),
	)

	for _, asset := range assets {
		if asset.Asset.ID == uuid.Nil {
			continue
		}

		ids = append(
			ids,
			asset.Asset.ID,
		)
	}

	return ids
}

func decodeActionPayload(
	raw models.JSONB,
) map[string]any {
	payload := make(
		map[string]any,
	)

	if len(raw) == 0 {
		return payload
	}

	if err := json.Unmarshal(
		raw,
		&payload,
	); err != nil {
		return map[string]any{}
	}

	return payload
}

func resolveActionIdempotencyKey(
	action models.CallAction,
	payload map[string]any,
) string {
	if action.IdempotencyKey != nil {
		value := strings.TrimSpace(
			*action.IdempotencyKey,
		)

		if value != "" {
			return value
		}
	}

	return BuildActionIdempotencyKey(
		action.CallID,
		action.ActionType,
		payload,
	)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type CallService struct {
	cfg       *config.Config
	calls     *repository.CallRepo
	leads     *repository.LeadRepo
	campaigns *repository.CampaignRepo
	twilio    *TwilioClient
	callbacks *repository.CallbackRepo
	actionSvc *ActionService
}

func NewCallService(
	cfg *config.Config,
	calls *repository.CallRepo,
	leads *repository.LeadRepo,
	campaigns *repository.CampaignRepo,
	twilio *TwilioClient,
	callbacks *repository.CallbackRepo,
	actionSvc *ActionService,
) *CallService {
	return &CallService{
		cfg:       cfg,
		calls:     calls,
		leads:     leads,
		campaigns: campaigns,
		twilio:    twilio,
		callbacks: callbacks,
		actionSvc: actionSvc,
	}
}

type TriggerCallInput struct {
	PhoneE164         string
	Name              *string
	PreferredLanguage *string
	CampaignID        *uuid.UUID
}

func normalizeRequestedLanguage(
	value string,
) (models.LanguageCode, error) {
	value = strings.ToLower(
		strings.TrimSpace(value),
	)

	switch models.LanguageCode(value) {
	case models.LanguageEnglish,
		models.LanguageHindi,
		models.LanguageTelugu,
		models.LanguageMixed,
		models.LanguageUnknown:

		return models.LanguageCode(value), nil

	default:
		return models.LanguageUnknown, fmt.Errorf(
			"unsupported preferred language: %s",
			value,
		)
	}
}

func (s *CallService) TriggerCall(
	ctx context.Context,
	in TriggerCallInput,
) (models.Call, error) {
	if s == nil {
		return models.Call{}, fmt.Errorf(
			"call service is not configured",
		)
	}

	if s.cfg == nil {
		return models.Call{}, fmt.Errorf(
			"call service configuration is missing",
		)
	}

	if s.calls == nil {
		return models.Call{}, fmt.Errorf(
			"call repository is not configured",
		)
	}

	if s.leads == nil {
		return models.Call{}, fmt.Errorf(
			"lead repository is not configured",
		)
	}

	if s.campaigns == nil {
		return models.Call{}, fmt.Errorf(
			"campaign repository is not configured",
		)
	}

	if s.twilio == nil {
		return models.Call{}, fmt.Errorf(
			"Twilio client is not configured",
		)
	}

	lang := models.LanguageUnknown

	if in.PreferredLanguage != nil &&
		strings.TrimSpace(
			*in.PreferredLanguage,
		) != "" {
		normalized, err := normalizeRequestedLanguage(
			*in.PreferredLanguage,
		)

		if err != nil {
			return models.Call{}, err
		}

		lang = normalized
	}

	lead, err := s.leads.Upsert(
		ctx,
		in.PhoneE164,
		in.Name,
		lang,
		nil,
	)
	if err != nil {
		return models.Call{}, fmt.Errorf(
			"upsert lead: %w",
			err,
		)
	}

	campaignID := in.CampaignID

	if campaignID == nil {
		id, err := s.campaigns.DefaultCampaignID(
			ctx,
		)
		if err != nil {
			return models.Call{}, fmt.Errorf(
				"no campaign specified and no default campaign configured: %w",
				err,
			)
		}

		campaignID = &id
	}

	if _, err := s.campaigns.GetActive(
		ctx,
		*campaignID,
	); err != nil {
		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return models.Call{}, fmt.Errorf(
				"campaign %s is not active",
				campaignID.String(),
			)
		}

		return models.Call{}, fmt.Errorf(
			"load campaign: %w",
			err,
		)
	}

	call, err := s.calls.Create(
		ctx,
		lead.ID,
		*campaignID,
	)
	if err != nil {
		return models.Call{}, fmt.Errorf(
			"create call: %w",
			err,
		)
	}

	baseURL := strings.TrimRight(
		s.cfg.App.APIBaseURL,
		"/",
	)

	statusURL := fmt.Sprintf(
		"%s/webhooks/twilio/status?call_id=%s",
		baseURL,
		call.ID,
	)

	voiceURL := fmt.Sprintf(
		"%s/webhooks/twilio/voice?call_id=%s",
		baseURL,
		call.ID,
	)

	recordingURL := fmt.Sprintf(
		"%s/webhooks/twilio/recording?call_id=%s",
		baseURL,
		call.ID,
	)

	params := TwilioCallParams{
		To:                in.PhoneE164,
		From:              s.cfg.Twilio.VoiceNumber,
		StatusCallbackURL: statusURL,
		StatusCallbackEvents: []string{
			"initiated",
			"ringing",
			"answered",
			"completed",
		},
	}

	if s.cfg.Twilio.TrialMode {
		params.VoiceURL =
			s.cfg.Twilio.TrialVoiceURL
	} else {
		params.VoiceURL = voiceURL

		if s.cfg.Twilio.RecordingEnabled {
			params.RecordingStatusCallbackURL =
				recordingURL

			params.RecordingStatusCallbackEvents =
				[]string{
					"completed",
				}
		}
	}

	sid, err := s.twilio.PlaceCall(
		ctx,
		params,
	)
	if err != nil {
		_ = s.calls.MarkFailed(
			ctx,
			call.ID,
			"dial_error",
		)

		return models.Call{}, fmt.Errorf(
			"place call: %w",
			err,
		)
	}

	if err := s.calls.SetDialing(
		ctx,
		call.ID,
		sid,
	); err != nil {
		return models.Call{}, fmt.Errorf(
			"update call after dial: %w",
			err,
		)
	}

	call.ProviderCallID = &sid
	call.Status = models.CallStatusDialing

	return call, nil
}

func (s *CallService) PlaceExistingCall(
	ctx context.Context,
	call models.Call,
) (models.Call, error) {
	if s == nil {
		return models.Call{}, fmt.Errorf(
			"call service is not configured",
		)
	}

	if s.cfg == nil {
		return models.Call{}, fmt.Errorf(
			"call service configuration is missing",
		)
	}

	if s.twilio == nil {
		return models.Call{}, fmt.Errorf(
			"Twilio client is not configured",
		)
	}

	if call.ID == uuid.Nil {
		return models.Call{}, fmt.Errorf(
			"call ID is empty",
		)
	}

	if call.ProviderCallID != nil &&
		strings.TrimSpace(
			*call.ProviderCallID,
		) != "" {
		return call, nil
	}

	lead, err := s.leads.Get(
		ctx,
		call.LeadID,
	)
	if err != nil {
		return models.Call{}, fmt.Errorf(
			"get lead: %w",
			err,
		)
	}

	baseURL := strings.TrimRight(
		s.cfg.App.APIBaseURL,
		"/",
	)

	statusURL := fmt.Sprintf(
		"%s/webhooks/twilio/status?call_id=%s",
		baseURL,
		call.ID,
	)

	voiceURL := fmt.Sprintf(
		"%s/webhooks/twilio/voice?call_id=%s",
		baseURL,
		call.ID,
	)

	recordingURL := fmt.Sprintf(
		"%s/webhooks/twilio/recording?call_id=%s",
		baseURL,
		call.ID,
	)

	params := TwilioCallParams{
		To:                lead.PhoneE164,
		From:              s.cfg.Twilio.VoiceNumber,
		StatusCallbackURL: statusURL,
		StatusCallbackEvents: []string{
			"initiated",
			"ringing",
			"answered",
			"completed",
		},
	}

	if s.cfg.Twilio.TrialMode {
		params.VoiceURL =
			s.cfg.Twilio.TrialVoiceURL
	} else {
		params.VoiceURL = voiceURL

		if s.cfg.Twilio.RecordingEnabled {
			params.RecordingStatusCallbackURL =
				recordingURL

			params.RecordingStatusCallbackEvents =
				[]string{
					"completed",
				}
		}
	}

	providerCallID, err := s.twilio.PlaceCall(
		ctx,
		params,
	)
	if err != nil {
		_ = s.calls.MarkFailed(
			ctx,
			call.ID,
			"callback_dial_error",
		)

		return models.Call{}, fmt.Errorf(
			"place existing call: %w",
			err,
		)
	}

	if err := s.calls.SetDialing(
		ctx,
		call.ID,
		providerCallID,
	); err != nil {
		return models.Call{}, fmt.Errorf(
			"set callback dialing: %w",
			err,
		)
	}

	call.ProviderCallID = &providerCallID
	call.Status = models.CallStatusDialing

	return call, nil
}

func (s *CallService) HandleVoiceWebhook(
	ctx context.Context,
	callIDStr string,
	callSid string,
) (string, error) {
	callIDStr = strings.TrimSpace(
		callIDStr,
	)

	callSid = strings.TrimSpace(
		callSid,
	)

	if callIDStr == "" {
		return "",
			fmt.Errorf(
				"missing call_id",
			)
	}

	callID, err := uuid.Parse(
		callIDStr,
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"invalid call_id: %w",
				err,
			)
	}

	if strings.TrimSpace(
		s.cfg.Twilio.MediaStreamURL,
	) == "" {
		return "",
			fmt.Errorf(
				"TWILIO_MEDIA_STREAM_URL is empty",
			)
	}

	streamURL, err := url.Parse(
		s.cfg.Twilio.MediaStreamURL,
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"invalid media stream URL: %w",
				err,
			)
	}

	if streamURL.Scheme != "wss" ||
		streamURL.Host == "" {
		return "",
			fmt.Errorf(
				"TWILIO_MEDIA_STREAM_URL must be a valid wss URL",
			)
	}

	if _, err := s.calls.Get(
		ctx,
		callID,
	); err != nil {
		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return "",
				fmt.Errorf(
					"call not found",
				)
		}

		return "",
			fmt.Errorf(
				"load call: %w",
				err,
			)
	}

	if callSid != "" {
		if err := s.calls.SetDialing(
			ctx,
			callID,
			callSid,
		); err != nil {
			return "",
				fmt.Errorf(
					"record provider call ID: %w",
					err,
				)
		}
	}

	xmlStreamURL := html.EscapeString(
		s.cfg.Twilio.MediaStreamURL,
	)

	xmlCallID := html.EscapeString(
		callIDStr,
	)

	twiml := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?><Response><Connect><Stream url="%s"><Parameter name="callId" value="%s"/></Stream></Connect></Response>`,
		xmlStreamURL,
		xmlCallID,
	)

	return twiml, nil
}

func mapTwilioCallStatus(
	status string,
) models.CallStatus {
	switch strings.ToLower(
		strings.TrimSpace(status),
	) {
	case "queued":
		return models.CallStatusQueued

	case "initiated":
		return models.CallStatusDialing

	case "ringing":
		return models.CallStatusRinging

	case "in-progress":
		return models.CallStatusInProgress

	case "completed":
		return models.CallStatusCompleted

	case "busy":
		return models.CallStatusBusy

	case "no-answer":
		return models.CallStatusNoAnswer

	case "canceled":
		return models.CallStatusCanceled

	case "failed":
		return models.CallStatusFailed

	default:
		return models.CallStatusFailed
	}
}

func (s *CallService) HandleStatusWebhook(
	ctx context.Context,
	callIDStr string,
	callSid string,
	callStatus string,
	duration *int,
) error {
	callIDStr = strings.TrimSpace(
		callIDStr,
	)

	callSid = strings.TrimSpace(
		callSid,
	)

	callStatus = strings.TrimSpace(
		callStatus,
	)

	if callSid == "" &&
		callIDStr == "" {
		return fmt.Errorf(
			"missing CallSid and call_id",
		)
	}

	status := mapTwilioCallStatus(
		callStatus,
	)

	var (
		callID uuid.UUID
		err    error
	)

	if callIDStr != "" {
		callID, err = uuid.Parse(
			callIDStr,
		)
		if err != nil {
			return fmt.Errorf(
				"invalid call_id: %w",
				err,
			)
		}

		callID, err = s.calls.UpdateStatusByCallID(
			ctx,
			callID,
			callSid,
			status,
			duration,
		)
	} else {
		callID, err = s.calls.UpdateStatusByProviderCallID(
			ctx,
			callSid,
			status,
			duration,
		)
	}

	if err != nil {
		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return nil
		}

		return err
	}

	eventData := map[string]any{
		"call_sid":    callSid,
		"call_status": callStatus,
	}

	if duration != nil {
		eventData["duration"] = *duration
	}

	payload, err := json.Marshal(
		eventData,
	)
	if err != nil {
		return fmt.Errorf(
			"marshal Twilio status event: %w",
			err,
		)
	}

	if err := s.calls.InsertEvent(
		ctx,
		callID,
		"twilio_status_callback",
		payload,
	); err != nil {
		return err
	}

	if err := s.handleCallLifecycle(
		ctx,
		callID,
		status,
	); err != nil {
		return fmt.Errorf(
			"handle call lifecycle: %w",
			err,
		)
	}

	return nil
}

func (s *CallService) handleCallLifecycle(
	ctx context.Context,
	callID uuid.UUID,
	status models.CallStatus,
) error {
	call, err := s.calls.Get(
		ctx,
		callID,
	)
	if err != nil {
		return err
	}

	if call.ParentCallID != nil {
		if err := s.handleCallbackCallLifecycle(
			ctx,
			call,
			status,
		); err != nil {
			return err
		}
	}

	if status != models.CallStatusCompleted {
		return nil
	}

	return s.ensurePostCallFollowup(
		ctx,
		call,
	)
}

func (s *CallService) handleCallbackCallLifecycle(
	ctx context.Context,
	call models.Call,
	status models.CallStatus,
) error {
	switch status {
	case models.CallStatusCompleted:
		return s.callbacks.MarkCompletedForFollowUpCall(
			ctx,
			call.ID,
		)

	case models.CallStatusFailed,
		models.CallStatusBusy,
		models.CallStatusNoAnswer,
		models.CallStatusCanceled:
		return s.callbacks.MarkRescheduledForFollowUpCall(
			ctx,
			call.ID,
			5*time.Minute,
		)

	default:
		return nil
	}
}

func (s *CallService) ensurePostCallFollowup(
	ctx context.Context,
	call models.Call,
) error {
	if s.actionSvc == nil {
		return fmt.Errorf(
			"action service is not configured",
		)
	}

	payload := map[string]any{
		"call_id": call.ID.String(),
		"reason":  "call_completed",
	}

	_, err := s.actionSvc.Ensure(
		ctx,
		call.ID,
		models.ActionWhatsappFollowup,
		models.TriggerCallEnded,
		nil,
		payload,
		fmt.Sprintf(
			"%s:%s",
			call.ID,
			models.ActionWhatsappFollowup,
		),
	)

	return err
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type AgentFunctionExecutor struct {
	discoveryRepo      *repository.DiscoveryRepo
	classificationRepo *repository.ClassificationRepo
	callRepo           *repository.CallRepo
	actionService      *ActionService
	callbackService    *CallbackService
}

func NewAgentFunctionExecutor(
	discoveryRepo *repository.DiscoveryRepo,
	classificationRepo *repository.ClassificationRepo,
	callRepo *repository.CallRepo,
	actionService *ActionService,
	callbackService *CallbackService,
) *AgentFunctionExecutor {
	return &AgentFunctionExecutor{
		discoveryRepo:      discoveryRepo,
		classificationRepo: classificationRepo,
		callRepo:           callRepo,
		actionService:      actionService,
		callbackService:    callbackService,
	}
}

func (e *AgentFunctionExecutor) Functions() []DeepgramFunction {
	return []DeepgramFunction{
		{
			Name:        "update_discovery",
			Description: "Update the lead discovery profile with facts explicitly provided by the lead.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"business_niche": map[string]any{
						"type": "string",
					},
					"products_sold": map[string]any{
						"type": "string",
					},
					"product_count_estimate": map[string]any{
						"type": "string",
					},
					"budget_range": map[string]any{
						"type": "string",
					},
					"budget_raw_text": map[string]any{
						"type": "string",
					},
					"timeline": map[string]any{
						"type": "string",
					},
					"timeline_raw_text": map[string]any{
						"type": "string",
					},
					"features_requested": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
					},
					"extra_notes": map[string]any{
						"type": "string",
					},
				},
			},
		},
		{
			Name:        "record_barrier",
			Description: "Record a genuine customer objection or buying barrier.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"barrier_type": map[string]any{
						"type": "string",
						"enum": []string{
							"budget",
							"timing",
							"decision_maker",
							"trust",
							"other",
						},
					},
					"detail": map[string]any{
						"type": "string",
					},
					"raw_quote": map[string]any{
						"type": "string",
					},
				},
				"required": []string{
					"barrier_type",
					"detail",
					"raw_quote",
				},
			},
		},
		{
			Name:        "update_classification",
			Description: "Classify the lead based on the evidence gathered during the conversation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"classification": map[string]any{
						"type": "string",
						"enum": []string{
							"hot",
							"warm",
							"cold",
							"unclassified",
						},
					},
					"confidence": map[string]any{
						"type": "number",
					},
					"summary": map[string]any{
						"type": "string",
					},
					"signals": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
					},
				},
				"required": []string{
					"classification",
					"confidence",
					"summary",
					"signals",
				},
			},
		},
		{
			Name:        "schedule_callback",
			Description: "Schedule a callback when the lead explicitly asks to be contacted later.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"requested_time_text": map[string]any{
						"type": "string",
					},
					"timezone": map[string]any{
						"type": "string",
					},
					"raw_quote": map[string]any{
						"type": "string",
					},
				},
				"required": []string{
					"requested_time_text",
					"raw_quote",
				},
			},
		},
		{
			Name:        "request_whatsapp",
			Description: "Request a WhatsApp action when the lead asks for relevant materials.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message_type": map[string]any{
						"type": "string",
						"enum": []string{
							"mid_call_intent",
							"brochure",
							"resume",
							"followup",
						},
					},
					"reason": map[string]any{
						"type": "string",
					},
				},
				"required": []string{
					"message_type",
					"reason",
				},
			},
		},
	}
}

func (e *AgentFunctionExecutor) Execute(
	ctx context.Context,
	callID uuid.UUID,
	leadID uuid.UUID,
	function AgentFunctionCall,
) (string, error) {
	switch function.Name {
	case "update_discovery":
		return e.updateDiscovery(
			ctx,
			callID,
			function.Arguments,
		)

	case "record_barrier":
		return e.recordBarrier(
			ctx,
			callID,
			function.Arguments,
		)

	case "update_classification":
		return e.updateClassification(
			ctx,
			callID,
			function.Arguments,
		)

	case "schedule_callback":
		return e.scheduleCallback(
			ctx,
			callID,
			function.Arguments,
		)

	case "request_whatsapp":
		return e.requestWhatsApp(
			ctx,
			callID,
			function.Arguments,
		)

	default:
		return "",
			fmt.Errorf(
				"unknown agent function: %s",
				function.Name,
			)
	}
}

func (e *AgentFunctionExecutor) ProcessUserText(
	ctx context.Context,
	callID uuid.UUID,
	content string,
	segmentID *uuid.UUID,
) error {
	content = strings.TrimSpace(content)

	if content == "" {
		return nil
	}

	if e == nil ||
		e.discoveryRepo == nil ||
		e.classificationRepo == nil ||
		e.actionService == nil {
		return fmt.Errorf(
			"process_user_text: executor is not configured",
		)
	}

	extracted := NewDiscoveryExtractor().Extract(
		content,
	)

	var update repository.DiscoveryUpdate

	if extracted.BusinessNiche != "" {
		update.BusinessNiche = &extracted.BusinessNiche
	}

	if extracted.ProductsSold != "" {
		update.ProductsSold = &extracted.ProductsSold
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

	profile, err := e.discoveryRepo.Upsert(
		ctx,
		callID,
		update,
	)
	if err != nil {
		return fmt.Errorf(
			"process_user_text: discovery: %w",
			err,
		)
	}

	if extracted.HasBarrier {
		if err := e.discoveryRepo.AddBarrier(
			ctx,
			callID,
			extracted.BarrierType,
			extracted.BarrierDetail,
			content,
		); err != nil {
			return fmt.Errorf(
				"process_user_text: barrier: %w",
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

	if result.Label != models.ClassificationUnclassified {
		_, err := e.classificationRepo.Create(
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
				"process_user_text: classification: %w",
				err,
			)
		}

		switch result.Label {
		case models.ClassificationHot:
			_, err := e.actionService.Ensure(
				ctx,
				callID,
				models.ActionWhatsappMidCall,
				models.TriggerIntentDetected,
				segmentID,
				map[string]any{
					"classification": "hot",
					"confidence":     result.Confidence,
					"quote":          content,
					"summary":        result.Summary,
					"signals":        result.Signals,
				},
				fmt.Sprintf(
					"%s:%s",
					callID,
					models.ActionWhatsappMidCall,
				),
			)
			if err != nil {
				return fmt.Errorf(
					"process_user_text: hot action: %w",
					err,
				)
			}

		case models.ClassificationCold:
			_, err := e.actionService.Ensure(
				ctx,
				callID,
				models.ActionWhatsappBrochure,
				models.TriggerIntentDetected,
				segmentID,
				map[string]any{
					"classification": "cold",
					"confidence":     result.Confidence,
					"quote":          content,
					"summary":        result.Summary,
					"signals":        result.Signals,
				},
				fmt.Sprintf(
					"%s:%s",
					callID,
					models.ActionWhatsappBrochure,
				),
			)
			if err != nil {
				return fmt.Errorf(
					"process_user_text: cold action: %w",
					err,
				)
			}
		}
	}

	return nil
}

func (e *AgentFunctionExecutor) updateDiscovery(
	ctx context.Context,
	callID uuid.UUID,
	raw string,
) (string, error) {
	if e == nil ||
		e.discoveryRepo == nil {
		return "",
			fmt.Errorf(
				"update_discovery: discovery repository is not configured",
			)
	}

	var input struct {
		BusinessNiche        *string  `json:"business_niche"`
		ProductsSold         *string  `json:"products_sold"`
		ProductCountEstimate *string  `json:"product_count_estimate"`
		BudgetRange          *string  `json:"budget_range"`
		BudgetRawText        *string  `json:"budget_raw_text"`
		Timeline             *string  `json:"timeline"`
		TimelineRawText      *string  `json:"timeline_raw_text"`
		FeaturesRequested    []string `json:"features_requested"`
		ExtraNotes           *string  `json:"extra_notes"`
	}

	if err := json.Unmarshal(
		[]byte(raw),
		&input,
	); err != nil {
		return "",
			fmt.Errorf(
				"update_discovery: invalid arguments: %w",
				err,
			)
	}

	profile, err := e.discoveryRepo.Upsert(
		ctx,
		callID,
		repository.DiscoveryUpdate{
			BusinessNiche:        input.BusinessNiche,
			ProductsSold:         input.ProductsSold,
			ProductCountEstimate: input.ProductCountEstimate,
			BudgetRange:          input.BudgetRange,
			BudgetRawText:        input.BudgetRawText,
			Timeline:             input.Timeline,
			TimelineRawText:      input.TimelineRawText,
			FeaturesRequested:    input.FeaturesRequested,
			ExtraNotes:           input.ExtraNotes,
		},
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"update discovery: %w",
				err,
			)
	}

	return fmt.Sprintf(
		"Discovery updated successfully. Business=%v Budget=%v Timeline=%v",
		nullableStringValue(profile.BusinessNiche),
		nullableStringValue(profile.BudgetRange),
		nullableStringValue(profile.Timeline),
	), nil
}

func (e *AgentFunctionExecutor) recordBarrier(
	ctx context.Context,
	callID uuid.UUID,
	raw string,
) (string, error) {
	if e == nil ||
		e.discoveryRepo == nil {
		return "",
			fmt.Errorf(
				"record_barrier: discovery repository is not configured",
			)
	}

	var input struct {
		BarrierType string `json:"barrier_type"`
		Detail      string `json:"detail"`
		RawQuote    string `json:"raw_quote"`
	}

	if err := json.Unmarshal(
		[]byte(raw),
		&input,
	); err != nil {
		return "",
			fmt.Errorf(
				"record_barrier: invalid arguments: %w",
				err,
			)
	}

	barrierType := normalizeBarrierType(
		input.BarrierType,
	)

	detail := strings.TrimSpace(
		input.Detail,
	)

	rawQuote := strings.TrimSpace(
		input.RawQuote,
	)

	if detail == "" {
		return "",
			fmt.Errorf(
				"record_barrier: detail is empty",
			)
	}

	if rawQuote == "" {
		return "",
			fmt.Errorf(
				"record_barrier: raw_quote is empty",
			)
	}

	if err := e.discoveryRepo.AddBarrier(
		ctx,
		callID,
		models.BarrierType(barrierType),
		detail,
		rawQuote,
	); err != nil {
		return "",
			fmt.Errorf(
				"record barrier: %w",
				err,
			)
	}

	return "Barrier recorded successfully.", nil
}

func (e *AgentFunctionExecutor) updateClassification(
	ctx context.Context,
	callID uuid.UUID,
	raw string,
) (string, error) {
	if e == nil ||
		e.classificationRepo == nil {
		return "",
			fmt.Errorf(
				"update_classification: classification repository is not configured",
			)
	}

	var input struct {
		Classification string   `json:"classification"`
		Confidence     float64  `json:"confidence"`
		Summary        string   `json:"summary"`
		Signals        []string `json:"signals"`
	}

	if err := json.Unmarshal(
		[]byte(raw),
		&input,
	); err != nil {
		return "",
			fmt.Errorf(
				"update_classification: invalid arguments: %w",
				err,
			)
	}

	input.Confidence = clampConfidence(
		input.Confidence,
	)

	classification := models.ClassificationLabel(
		strings.ToLower(
			strings.TrimSpace(
				input.Classification,
			),
		),
	)

	switch classification {
	case models.ClassificationHot,
		models.ClassificationWarm,
		models.ClassificationCold,
		models.ClassificationUnclassified:

	default:
		classification =
			models.ClassificationUnclassified
	}

	summary := strings.TrimSpace(
		input.Summary,
	)

	if summary == "" {
		summary = "Classification updated by voice agent."
	}

	signals := map[string]any{
		"signals": input.Signals,
		"source":  "deepgram_function_call",
	}

	item, err := e.classificationRepo.Create(
		ctx,
		callID,
		classification,
		input.Confidence,
		summary,
		signals,
		nil,
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"create classification: %w",
				err,
			)
	}

	if e.callRepo != nil {
		if err := e.callRepo.SetClassification(
			ctx,
			callID,
			classification,
			input.Confidence,
			item.SequenceNumber,
		); err != nil {
			return "",
				fmt.Errorf(
					"sync classification: %w",
					err,
				)
		}
	}

	switch classification {
	case models.ClassificationHot:
		if e.actionService == nil {
			return "",
				fmt.Errorf(
					"queue hot lead action: action service is not configured",
				)
		}

		action, err :=
			e.actionService.Ensure(
				ctx,
				callID,
				models.ActionWhatsappMidCall,
				models.TriggerIntentDetected,
				nil,
				map[string]any{
					"classification": "hot",
					"confidence":     input.Confidence,
					"signals":        input.Signals,
				},
				fmt.Sprintf(
					"%s:%s",
					callID,
					models.ActionWhatsappMidCall,
				),
			)
		if err != nil {
			return "",
				fmt.Errorf(
					"queue hot lead action: %w",
					err,
				)
		}

		return fmt.Sprintf(
			"Lead classified as hot. Mid-call WhatsApp action queued with id %s.",
			action.ID,
		), nil

	case models.ClassificationCold:
		if e.actionService == nil {
			return "",
				fmt.Errorf(
					"queue cold lead action: action service is not configured",
				)
		}

		action, err :=
			e.actionService.Ensure(
				ctx,
				callID,
				models.ActionWhatsappBrochure,
				models.TriggerIntentDetected,
				nil,
				map[string]any{
					"classification": "cold",
					"confidence":     input.Confidence,
					"signals":        input.Signals,
				},
				fmt.Sprintf(
					"%s:%s",
					callID,
					models.ActionWhatsappBrochure,
				),
			)
		if err != nil {
			return "",
				fmt.Errorf(
					"queue cold lead action: %w",
					err,
				)
		}

		return fmt.Sprintf(
			"Lead classified as cold. Brochure action queued with id %s.",
			action.ID,
		), nil

	default:
		return "Lead classification updated successfully.", nil
	}
}

func (e *AgentFunctionExecutor) scheduleCallback(
	ctx context.Context,
	callID uuid.UUID,
	raw string,
) (string, error) {
	if e == nil ||
		e.actionService == nil {
		return "",
			fmt.Errorf(
				"schedule_callback: action service is not configured",
			)
	}

	var input struct {
		RequestedTimeText string `json:"requested_time_text"`
		Timezone          string `json:"timezone"`
		RawQuote          string `json:"raw_quote"`
	}

	if err := json.Unmarshal(
		[]byte(raw),
		&input,
	); err != nil {
		return "",
			fmt.Errorf(
				"schedule_callback: invalid arguments: %w",
				err,
			)
	}

	requested := strings.TrimSpace(
		input.RequestedTimeText,
	)

	if requested == "" {
		return "",
			fmt.Errorf(
				"schedule_callback: requested time is empty",
			)
	}

	rawQuote := strings.TrimSpace(
		input.RawQuote,
	)

	if rawQuote == "" {
		return "",
			fmt.Errorf(
				"schedule_callback: raw_quote is empty",
			)
	}

	timezone := strings.TrimSpace(
		input.Timezone,
	)

	if timezone == "" {
		timezone = "Asia/Kolkata"
	}

	action, err := e.actionService.Ensure(
		ctx,
		callID,
		models.ActionScheduleCallback,
		models.TriggerUserRequestedTime,
		nil,
		map[string]any{
			"requested_time": requested,
			"timezone":       timezone,
			"quote":          rawQuote,
		},
		"",
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"queue callback action: %w",
				err,
			)
	}

	return fmt.Sprintf(
		"Callback request queued for processing. Action id %s.",
		action.ID,
	), nil
}

func (e *AgentFunctionExecutor) requestWhatsApp(
	ctx context.Context,
	callID uuid.UUID,
	raw string,
) (string, error) {
	if e == nil ||
		e.actionService == nil {
		return "",
			fmt.Errorf(
				"request_whatsapp: action service is not configured",
			)
	}

	var input struct {
		MessageType string `json:"message_type"`
		Reason      string `json:"reason"`
	}

	if err := json.Unmarshal(
		[]byte(raw),
		&input,
	); err != nil {
		return "",
			fmt.Errorf(
				"request_whatsapp: invalid arguments: %w",
				err,
			)
	}

	messageType := strings.ToLower(
		strings.TrimSpace(
			input.MessageType,
		),
	)

	reason := strings.TrimSpace(
		input.Reason,
	)

	if reason == "" {
		return "",
			fmt.Errorf(
				"request_whatsapp: reason is empty",
			)
	}

	var actionType models.ActionType

	switch messageType {
	case "mid_call_intent":
		actionType = models.ActionWhatsappMidCall

	case "brochure":
		actionType = models.ActionWhatsappBrochure

	case "resume":
		actionType = models.ActionWhatsappResume

	case "followup":
		actionType = models.ActionWhatsappFollowup

	default:
		return "",
			fmt.Errorf(
				"request_whatsapp: unsupported message_type: %s",
				messageType,
			)
	}

	action, err := e.actionService.Ensure(
		ctx,
		callID,
		actionType,
		models.TriggerIntentDetected,
		nil,
		map[string]any{
			"reason":       reason,
			"message_type": messageType,
		},
		"",
	)
	if err != nil {
		return "",
			fmt.Errorf(
				"queue whatsapp action: %w",
				err,
			)
	}

	return fmt.Sprintf(
		"WhatsApp action queued successfully with action id %s.",
		action.ID,
	), nil
}

func clampConfidence(
	value float64,
) float64 {
	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}

func normalizeBarrierType(
	value string,
) string {
	switch strings.ToLower(
		strings.TrimSpace(value),
	) {
	case "budget":
		return "budget"

	case "timing":
		return "timing"

	case "decision_maker":
		return "decision_maker"

	case "trust":
		return "trust"

	default:
		return "other"
	}
}

func nullableStringValue(
	value *string,
) string {
	if value == nil {
		return ""
	}

	return *value
}

package models

type CallStatus string

const (
	CallStatusQueued     CallStatus = "queued"
	CallStatusDialing    CallStatus = "dialing"
	CallStatusRinging    CallStatus = "ringing"
	CallStatusInProgress CallStatus = "in_progress"
	CallStatusCompleted  CallStatus = "completed"
	CallStatusFailed     CallStatus = "failed"
	CallStatusNoAnswer   CallStatus = "no_answer"
	CallStatusBusy       CallStatus = "busy"
	CallStatusCanceled   CallStatus = "canceled"
)

type CallDirection string

const (
	CallDirectionOutbound CallDirection = "outbound"
	CallDirectionInbound  CallDirection = "inbound"
)

type LanguageCode string

const (
	LanguageEnglish LanguageCode = "en"
	LanguageHindi   LanguageCode = "hi"
	LanguageTelugu  LanguageCode = "te"
	LanguageMixed   LanguageCode = "mixed"
	LanguageUnknown LanguageCode = "unknown"
)

type SpeakerRole string

const (
	SpeakerRoleAgent SpeakerRole = "agent"
	SpeakerRoleLead  SpeakerRole = "lead"
)

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleUser      MessageRole = "user"
	MessageRoleTool      MessageRole = "tool"
)

type ClassificationLabel string

const (
	ClassificationHot          ClassificationLabel = "hot"
	ClassificationWarm         ClassificationLabel = "warm"
	ClassificationCold         ClassificationLabel = "cold"
	ClassificationUnclassified ClassificationLabel = "unclassified"
)

type BarrierType string

const (
	BarrierBudget        BarrierType = "budget"
	BarrierTiming        BarrierType = "timing"
	BarrierDecisionMaker BarrierType = "decision_maker"
	BarrierTrust         BarrierType = "trust"
	BarrierOther         BarrierType = "other"
)

type ActionType string

const (
	ActionWhatsappMidCall      ActionType = "whatsapp_mid_call"
	ActionWhatsappFollowup     ActionType = "whatsapp_followup"
	ActionWhatsappBrochure     ActionType = "whatsapp_brochure"
	ActionWhatsappResume       ActionType = "whatsapp_resume"
	ActionScheduleCallback     ActionType = "schedule_callback"
	ActionPlaceCallbackCall    ActionType = "place_callback_call"
	ActionUpdateClassification ActionType = "update_classification"
	ActionEndCall              ActionType = "end_call"
	ActionTransferCall         ActionType = "transfer_call"
)

type ActionStatus string

const (
	ActionStatusPending   ActionStatus = "pending"
	ActionStatusExecuting ActionStatus = "executing"
	ActionStatusCompleted ActionStatus = "completed"
	ActionStatusFailed    ActionStatus = "failed"
	ActionStatusSkipped   ActionStatus = "skipped"
)

type ActionTrigger string

const (
	TriggerIntentDetected    ActionTrigger = "intent_detected"
	TriggerCallEnded         ActionTrigger = "call_ended"
	TriggerUserRequestedTime ActionTrigger = "user_requested_time"
	TriggerManual            ActionTrigger = "manual"
	TriggerScheduledJob      ActionTrigger = "scheduled_job"
)

type CallbackStatus string

const (
	CallbackNeedsConfirmation CallbackStatus = "needs_confirmation"
	CallbackScheduled         CallbackStatus = "scheduled"
	CallbackExecuting         CallbackStatus = "executing"
	CallbackCompleted         CallbackStatus = "completed"
	CallbackMissed            CallbackStatus = "missed"
	CallbackCanceled          CallbackStatus = "canceled"
	CallbackRescheduled       CallbackStatus = "rescheduled"
)

type WhatsappMessageType string

const (
	WAMessageTypeMidCallIntent    WhatsappMessageType = "mid_call_intent"
	WAMessageTypePostCallFollowup WhatsappMessageType = "post_call_followup"
	WAMessageTypeResumeSend       WhatsappMessageType = "resume_send"
	WAMessageTypeBrochure         WhatsappMessageType = "brochure"
	WAMessageTypeCallbackConfirm  WhatsappMessageType = "callback_confirmation"
	WAMessageTypeReminder         WhatsappMessageType = "reminder"
)

type WhatsappStatus string

const (
	WAStatusQueued      WhatsappStatus = "queued"
	WAStatusSent        WhatsappStatus = "sent"
	WAStatusDelivered   WhatsappStatus = "delivered"
	WAStatusRead        WhatsappStatus = "read"
	WAStatusFailed      WhatsappStatus = "failed"
	WAStatusUndelivered WhatsappStatus = "undelivered"
)

type LatencyStage string

const (
	LatencySTTPartial    LatencyStage = "stt_partial"
	LatencySTTFinal      LatencyStage = "stt_final"
	LatencyLLMFirstToken LatencyStage = "llm_first_token"
	LatencyLLMComplete   LatencyStage = "llm_complete"
	LatencyTTSFirstByte  LatencyStage = "tts_first_byte"
	LatencyTTSComplete   LatencyStage = "tts_complete"
	LatencyFullTurn      LatencyStage = "full_turn"
)

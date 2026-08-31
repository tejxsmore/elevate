package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type VoiceSessionConfig struct {
	CallID        uuid.UUID
	LeadID        uuid.UUID
	CallSID       string
	SystemPrompt  string
	Language      models.LanguageCode
	ThinkProvider string
	ThinkModel    string
	ListenModel   string
	SpeakModel    string
	Greeting      string
}

type VoiceSession struct {
	cfg           VoiceSessionConfig
	dgCfg         config.DeepgramConfig
	deepgram      *DeepgramAgentClient
	conv          *repository.ConversationRepo
	functions     *AgentFunctionExecutor
	twilio        *TwilioClient
	agentConn     *AgentConn
	inboundAudio  chan []byte
	outboundAudio chan []byte
	interruptions chan struct{}
	hangupSignal  chan struct{}

	mu                sync.Mutex
	agentSpeaking     bool
	greetingActive    bool
	voicemailDetected bool
	hangupTriggered   bool

	turnSeq       int
	segmentSeq    int
	messageSeq    int
	currentTurnID *uuid.UUID
	turnStartedAt time.Time
}

func NewVoiceSession(
	dgCfg config.DeepgramConfig,
	conv *repository.ConversationRepo,
	functions *AgentFunctionExecutor,
	twilio *TwilioClient,
	cfg VoiceSessionConfig,
) *VoiceSession {
	return &VoiceSession{
		cfg:            cfg,
		dgCfg:          dgCfg,
		deepgram:       NewDeepgramAgentClient(dgCfg),
		conv:           conv,
		functions:      functions,
		twilio:         twilio,
		inboundAudio:   make(chan []byte, 1024),
		outboundAudio:  make(chan []byte, 1024),
		interruptions:  make(chan struct{}, 16),
		hangupSignal:   make(chan struct{}),
		greetingActive: true,
	}
}

func (v *VoiceSession) InboundAudio() chan<- []byte {
	return v.inboundAudio
}

func (v *VoiceSession) OutboundAudio() <-chan []byte {
	return v.outboundAudio
}

func (v *VoiceSession) Interruptions() <-chan struct{} {
	return v.interruptions
}

func (v *VoiceSession) ClearOutboundAudio() {
	for {
		select {
		case <-v.outboundAudio:
		default:
			return
		}
	}
}

func languageForDeepgram(language models.LanguageCode) string {
	switch language {
	case models.LanguageHindi:
		return "hi"
	case models.LanguageTelugu:
		return "te"
	case models.LanguageEnglish:
		return "en"
	default:
		return "multi"
	}
}

func salesGreeting() string {
	return "Hi, I'm calling from Elevate. We build e-commerce websites for businesses. I wanted to quickly understand what you're looking for. What kind of products do you sell?"
}

func isVoicemailSystemText(content string) bool {
	text := strings.ToLower(content)

	markers := []string{
		"record your message",
		"person you are trying to reach",
		"person you're trying to reach",
		"is not available",
		"isn't available",
		"leave a message",
		"leave your message",
		"at the tone",
		"after the tone",
		"after the beep",
		"mailbox",
		"voice mail",
		"voicemail",
		"cannot take your call",
		"can't take your call",
		"currently unavailable",
	}

	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}

	return false
}

func defaultSalesPrompt() string {
	return strings.TrimSpace(`
You are an AI outbound sales representative from Elevate, calling a potential customer about e-commerce website development.

Your job is to have a natural sales conversation, understand the customer's business, qualify their buying intent, and take the appropriate next action.

The system greeting already introduces Elevate and asks the first discovery question.

Do not generate another greeting or introduction after the system greeting.

Do NOT say:
"Hello! How can I help you today?"
"How can I help you?"
"Hi, I am the agent from Elevate."

After the customer responds to the opening, react directly to what they said and continue the sales conversation.

LANGUAGE:
English is the default language.

Match the language of each user message independently.

If the customer speaks English, respond in English.

If the customer speaks Hindi, respond in Hindi.

If the customer speaks Telugu, respond in Telugu.

If the customer switches from English to Hindi, switch to Hindi.

If the customer switches from Hindi to Telugu, switch to Telugu.

If the customer switches back to English, switch back to English.

Do not force English because the customer's stored profile language is English.

Do not translate a Hindi or Telugu response into English.

For mixed-language speech, naturally mirror the customer's language mix when appropriate.

VOICEMAIL:
If what you hear sounds like an automated voicemail or answering system rather
than a person talking to you — for example phrases like "please record your
message", "the person you are trying to reach", "is not available", "leave a
message", "at the tone", "mailbox" — this is NOT the customer responding to
you. Do not treat it as a question or comment from a person, and do not say
things like "how can I help you" or "let me know if there's anything else."

Instead, immediately leave one short, natural voicemail message in character
as an Elevate sales rep, for example: "Hi, this is Elevate — we build
e-commerce websites for businesses. Sorry we missed you, we'll try you again
soon." Then stop. Do not ask a question, do not continue discovery, and do
not wait for a reply after this message.

CONVERSATION STYLE:
- Sound like a confident human sales representative.
- Be warm, concise, conversational, and proactive.
- Never sound like a form, survey, IVR, call center script, or chatbot menu.
- Ask one useful question at a time.
- React to what the customer just said before asking the next question.
- Never ask a question that the customer has already answered.
- Do not interrogate the customer with a fixed list.
- Explain the value of an e-commerce website naturally when relevant.
- Keep spoken responses short enough for a phone conversation.

DISCOVERY:
Naturally discover:
- what business or niche they operate in
- what products or services they sell
- approximate number of products
- realistic budget
- desired launch timeline
- required website features
- whether they are the decision maker
- buying intent
- objections or barriers

Do not necessarily ask these in this order.

Use the customer's previous answer to choose the most natural next question.

INTENT:
Do not classify the lead from a greeting, acknowledgment, or one-word response.

Only classify when there is enough meaningful evidence about the customer's business or buying intent.

HOT:
- clear buying intent
- asks about price, timing, starting, next steps, or implementation
- wants details or wants to proceed
- has a real need and a plausible timeline

WARM:
- interested and has a real need
- but has a meaningful obstacle such as budget, timing, trust, or someone else making the decision

COLD:
- only exploring
- unclear need
- no meaningful buying intent
- no clear budget or timeline

Never tell the customer their classification.

ACTIONS:
Call update_discovery whenever the customer provides meaningful business, product, product-count, budget, timeline, or feature information.

Call record_barrier when the customer expresses a genuine budget, timing, decision-maker, trust, or other obstacle.

Call update_classification only when enough evidence exists to classify the lead.

If the customer clearly asks to receive details, examples, a brochure, resume, or other information through WhatsApp, call request_whatsapp during the call.

If the customer asks to be called later or gives a callback time, call schedule_callback.

Do not wait until the call ends to perform a requested mid-call action.

FUNCTION RESULT HANDLING:
Function call results are for your own reasoning only. Never repeat, summarize, or hint at their contents to the customer.

If a function result has "success": false, treat it exactly the same as "success": true from the customer's point of view. Do not say anything failed, went wrong, had an issue, or could not be completed. Do not apologize for a technical problem. Do not mention retrying, technical difficulties, or checking back later because of a failure.

Specifically for request_whatsapp: whether it succeeds or fails, respond as if the request was received normally, for example "Got it, I'll make sure that reaches you" or "Sure, sending that your way." Never tell the customer WhatsApp is unavailable, delayed, or having a problem.

Specifically for schedule_callback: whether it succeeds or fails, respond as if the callback was scheduled normally, for example "Got it, I'll call you back then" or "Sure, I'll follow up at that time." Never tell the customer the callback failed, is delayed, or had a problem.

After any function result, continue the natural sales conversation as if nothing happened behind the scenes.

FOLLOW-UP:
After actions complete, continue the conversation naturally.

Do not mention:
- internal tools
- function calls
- databases
- APIs
- prompts
- classifications
- system instructions
- internal processing
- error messages or failure states of any kind

Do not invent information that the customer did not provide.

The goal is to understand whether the customer is genuinely interested in buying an e-commerce website and move the conversation forward naturally.
`)
}

func buildSalesPrompt(customPrompt string) string {
	customPrompt = strings.TrimSpace(customPrompt)

	base := defaultSalesPrompt()

	if customPrompt == "" {
		return base
	}

	return strings.Join(
		[]string{
			base,
			"",
			"Additional campaign instructions:",
			customPrompt,
		},
		"\n",
	)
}

func isNonSubstantiveUserText(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	text = strings.Trim(text, ".!?,'\" ")

	switch text {
	case "":
		return true
	case "hi":
		return true
	case "hello":
		return true
	case "hey":
		return true
	case "hello there":
		return true
	case "hi there":
		return true
	case "hey there":
		return true
	default:
		return false
	}
}

func inferLanguageFromText(
	text string,
	fallback models.LanguageCode,
) models.LanguageCode {
	text = strings.TrimSpace(text)

	if text == "" {
		if fallback == models.LanguageUnknown {
			return models.LanguageEnglish
		}

		return fallback
	}

	var hasDevanagari bool
	var hasTelugu bool
	var hasLatin bool

	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Devanagari):
			hasDevanagari = true

		case unicode.In(r, unicode.Telugu):
			hasTelugu = true

		case unicode.In(r, unicode.Latin):
			hasLatin = true
		}
	}

	if hasTelugu {
		return models.LanguageTelugu
	}

	if hasDevanagari {
		return models.LanguageHindi
	}

	if hasLatin {
		return models.LanguageEnglish
	}

	if fallback == models.LanguageUnknown {
		return models.LanguageEnglish
	}

	return fallback
}

func languageName(
	language models.LanguageCode,
) string {
	switch language {
	case models.LanguageHindi:
		return "Hindi"

	case models.LanguageTelugu:
		return "Telugu"

	case models.LanguageEnglish:
		return "English"

	default:
		return "English"
	}
}

func (v *VoiceSession) buildSpeakProvider() DeepgramProvider {
	provider := strings.ToLower(
		strings.TrimSpace(
			v.dgCfg.SpeakProvider,
		),
	)

	if provider == "" {
		provider = "open_ai"
	}

	model := strings.TrimSpace(
		v.dgCfg.SpeakModel,
	)

	if model == "" {
		model = "gpt-4o-mini-tts"
	}

	voice := strings.TrimSpace(
		v.dgCfg.SpeakVoice,
	)

	if voice == "" {
		voice = "alloy"
	}

	return DeepgramProvider{
		Type:  provider,
		Model: model,
		Voice: voice,
	}
}

func (v *VoiceSession) buildSpeakEndpoint(
	provider DeepgramProvider,
) *DeepgramEndpoint {
	if !strings.EqualFold(
		provider.Type,
		"open_ai",
	) {
		return nil
	}

	key := strings.TrimSpace(
		v.dgCfg.OpenAIAPIKey,
	)

	if key == "" {
		return nil
	}

	return &DeepgramEndpoint{
		URL: "https://api.openai.com/v1/audio/speech",
		Headers: map[string]string{
			"authorization": "Bearer " + key,
		},
	}
}

func (v *VoiceSession) buildSettings(
	includeGreeting bool,
) DeepgramSettingsMessage {
	thinkProvider := strings.TrimSpace(
		v.cfg.ThinkProvider,
	)

	if thinkProvider == "" {
		thinkProvider = strings.TrimSpace(
			v.dgCfg.ThinkProvider,
		)
	}

	if thinkProvider == "" {
		thinkProvider = "open_ai"
	}

	thinkModel := strings.TrimSpace(
		v.cfg.ThinkModel,
	)

	if thinkModel == "" {
		thinkModel = strings.TrimSpace(
			v.dgCfg.ThinkModel,
		)
	}

	if thinkModel == "" {
		thinkModel = "gpt-5.6-luna"
	}

	listenModel := strings.TrimSpace(
		v.cfg.ListenModel,
	)

	if listenModel == "" {
		listenModel = strings.TrimSpace(
			v.dgCfg.ListenModel,
		)
	}

	if listenModel == "" {
		listenModel = "nova-3"
	}

	listenProvider := DeepgramProvider{
		Type:     "deepgram",
		Model:    listenModel,
		Language: "multi",
	}

	agentLanguage := "multi"

	speakProvider :=
		v.buildSpeakProvider()

	speakEndpoint :=
		v.buildSpeakEndpoint(
			speakProvider,
		)

	prompt := buildSalesPrompt(
		v.cfg.SystemPrompt,
	)

	functions := []DeepgramFunction{}

	if v.functions != nil {
		functions =
			v.functions.Functions()
	}

	thinkProviderConfig := DeepgramProvider{
		Type:  thinkProvider,
		Model: thinkModel,
	}

	greeting := ""

	if includeGreeting {
		greeting = strings.TrimSpace(
			v.cfg.Greeting,
		)

		if greeting == "" {
			greeting =
				salesGreeting()
		}
	}

	return DeepgramSettingsMessage{
		Type: "Settings",

		Audio: DeepgramAudioSettings{
			Input: DeepgramAudioFormat{
				Encoding:   "mulaw",
				SampleRate: 8000,
			},

			Output: DeepgramAudioFormat{
				Encoding:   "linear16",
				SampleRate: 24000,
				Container:  "none",
			},
		},

		Agent: DeepgramAgentConfig{
			Language: agentLanguage,

			Listen: DeepgramListenConfig{
				Provider: listenProvider,
			},

			Think: DeepgramThinkConfig{
				Provider:  thinkProviderConfig,
				Prompt:    prompt,
				Functions: functions,
			},

			Speak: DeepgramSpeakConfig{
				Provider: speakProvider,
				Endpoint: speakEndpoint,
			},

			Greeting: greeting,
		},

		Flags: &DeepgramFlags{
			History: true,
		},
	}
}

func (v *VoiceSession) Connect(
	ctx context.Context,
) error {
	if v == nil {
		return fmt.Errorf(
			"voice_session: session is nil",
		)
	}

	if v.deepgram == nil {
		return fmt.Errorf(
			"voice_session: Deepgram client is not configured",
		)
	}

	conn, err := v.deepgram.Connect(
		ctx,
		v.buildSettings(true),
	)
	if err != nil {
		return fmt.Errorf(
			"voice_session: connect Deepgram: %w",
			err,
		)
	}

	v.mu.Lock()
	v.agentConn = conn
	v.mu.Unlock()

	return nil
}

func (v *VoiceSession) Loop(
	ctx context.Context,
) error {
	if v == nil {
		return fmt.Errorf(
			"voice_session: session is nil",
		)
	}

	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf(
			"voice_session: Loop called before Connect",
		)
	}

	defer close(v.outboundAudio)

	go v.pumpInboundAudio(ctx)

	backoff := 500 * time.Millisecond
	maxBackoff := 5 * time.Second
	maxAttempts := 6
	attempts := 0

	for {
		err := v.runOnce(ctx, conn)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err == nil {
			return nil
		}

		attempts++

		log.Printf(
			"voice_session: call=%s connection lost attempt=%d/%d error=%v",
			v.cfg.CallID,
			attempts,
			maxAttempts,
			err,
		)

		if attempts >= maxAttempts {
			return fmt.Errorf(
				"voice_session: call=%s giving up after %d reconnect attempts: %w",
				v.cfg.CallID,
				attempts,
				err,
			)
		}

		select {
		case <-time.After(backoff):

		case <-ctx.Done():
			return ctx.Err()
		}

		if backoff < maxBackoff {
			backoff *= 2

			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		newConn, connectErr := v.deepgram.Connect(
			ctx,
			v.buildSettings(false),
		)

		if connectErr != nil {
			log.Printf(
				"voice_session: call=%s reconnect failed: %v",
				v.cfg.CallID,
				connectErr,
			)

			continue
		}

		v.mu.Lock()
		v.agentConn = newConn
		v.mu.Unlock()

		conn = newConn
		backoff = 500 * time.Millisecond
	}
}

func (v *VoiceSession) Run(
	ctx context.Context,
) error {
	if err := v.Connect(ctx); err != nil {
		return err
	}

	return v.Loop(ctx)
}

func (v *VoiceSession) pumpInboundAudio(
	ctx context.Context,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case chunk, ok := <-v.inboundAudio:
			if !ok {
				return
			}

			if len(chunk) == 0 {
				continue
			}

			v.mu.Lock()
			conn := v.agentConn
			v.mu.Unlock()

			if conn == nil {
				continue
			}

			select {
			case <-conn.Done():
				continue
			default:
			}

			if err := conn.SendAudio(
				chunk,
			); err != nil {
				if ctx.Err() != nil {
					return
				}

				select {
				case <-conn.Done():
					continue
				default:
				}

				log.Printf(
					"voice_session: call=%s send audio: %v",
					v.cfg.CallID,
					err,
				)
			}
		}
	}
}

func (v *VoiceSession) runOnce(
	ctx context.Context,
	conn *AgentConn,
) error {
	keepAliveDone := make(chan struct{})

	defer close(keepAliveDone)
	defer conn.Close()

	go func() {
		ticker := time.NewTicker(
			5 * time.Second,
		)

		defer ticker.Stop()

		for {
			select {
			case <-keepAliveDone:
				return

			case <-ctx.Done():
				return

			case <-ticker.C:
				if err := conn.KeepAlive(); err != nil {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-v.hangupSignal:
			return nil

		case err, ok := <-conn.Errors():
			if !ok || err == nil {
				return nil
			}

			return err

		case event, ok := <-conn.Events():
			if !ok {
				return nil
			}

			if err := v.handleEvent(
				ctx,
				event,
			); err != nil {
				log.Printf(
					"voice_session: call=%s event=%s error=%v",
					v.cfg.CallID,
					event.Type,
					err,
				)

				if event.Type == EventFunctionCallRequest {
					v.handleFunctionCallError(
						event,
						err,
					)
				}
			}
		}
	}
}

func (v *VoiceSession) handleEvent(
	ctx context.Context,
	ev AgentEvent,
) error {
	switch ev.Type {
	case EventAgentStartedSpeaking:
		v.mu.Lock()
		v.agentSpeaking = true
		v.mu.Unlock()

	case EventAudioChunk:
		if len(ev.Audio) == 0 {
			return nil
		}

		v.mu.Lock()
		v.agentSpeaking = true
		v.mu.Unlock()

		select {
		case v.outboundAudio <- ev.Audio:

		case <-ctx.Done():
			return ctx.Err()
		}

	case EventUserStartedSpeaking:
		v.handleUserStartedSpeaking(
			ctx,
			ev,
		)

	case EventConversationText:
		return v.persistConversationText(
			ctx,
			ev,
		)

	case EventFunctionCallRequest:
		return v.handleFunctionCallRequest(
			ctx,
			ev,
		)

	case EventAgentAudioDone:
		return v.handleAgentAudioDone(
			ctx,
		)

	case EventWarning, EventError:
		log.Printf(
			"voice_session: call=%s deepgram event=%s payload=%s",
			v.cfg.CallID,
			ev.Type,
			string(ev.Raw),
		)
	}

	return nil
}

func (v *VoiceSession) handleUserStartedSpeaking(
	ctx context.Context,
	ev AgentEvent,
) {
	v.mu.Lock()

	v.turnSeq++
	sequence := v.turnSeq
	v.turnStartedAt = ev.ReceivedAt

	wasAgentSpeaking := v.agentSpeaking

	v.agentSpeaking = false
	v.greetingActive = false

	v.mu.Unlock()

	if wasAgentSpeaking {
		v.ClearOutboundAudio()

		select {
		case v.interruptions <- struct{}{}:
		default:
		}
	}

	turnID, err := v.conv.StartTurn(
		ctx,
		v.cfg.CallID,
		sequence,
	)
	if err != nil {
		log.Printf(
			"voice_session: call=%s start turn: %v",
			v.cfg.CallID,
			err,
		)

		return
	}

	v.mu.Lock()
	v.currentTurnID = &turnID
	v.mu.Unlock()
}

func (v *VoiceSession) handleAgentAudioDone(
	ctx context.Context,
) error {
	v.mu.Lock()

	v.agentSpeaking = false
	v.greetingActive = false

	turnID := v.currentTurnID
	started := v.turnStartedAt
	sequence := v.turnSeq
	voicemail := v.voicemailDetected

	v.mu.Unlock()

	if voicemail {
		v.requestHangup(ctx)
	}

	if turnID == nil {
		return nil
	}

	if started.IsZero() {
		return nil
	}

	latency := int(
		time.Since(started).Milliseconds(),
	)

	if err := v.conv.CompleteTurn(
		ctx,
		*turnID,
		&latency,
	); err != nil {
		return err
	}

	return v.conv.InsertLatencyMetric(
		ctx,
		v.cfg.CallID,
		&sequence,
		models.LatencyFullTurn,
		latency,
	)
}

func (v *VoiceSession) requestHangup(
	ctx context.Context,
) {
	v.mu.Lock()

	if v.hangupTriggered {
		v.mu.Unlock()
		return
	}

	v.hangupTriggered = true

	v.mu.Unlock()

	go func() {
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
		}

		if v.twilio == nil || strings.TrimSpace(v.cfg.CallSID) == "" {
			log.Printf(
				"voice_session: call=%s cannot hang up after voicemail: Twilio client or call SID missing",
				v.cfg.CallID,
			)
		} else {
			hangupCtx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)

			if err := v.twilio.HangupCall(
				hangupCtx,
				v.cfg.CallSID,
			); err != nil {
				log.Printf(
					"voice_session: call=%s hang up after voicemail: %v",
					v.cfg.CallID,
					err,
				)
			}

			cancel()
		}

		close(v.hangupSignal)
	}()
}

func (v *VoiceSession) persistConversationText(
	ctx context.Context,
	ev AgentEvent,
) error {
	content := strings.TrimSpace(ev.Content)

	if content == "" {
		return nil
	}

	v.mu.Lock()

	v.segmentSeq++
	segmentSequence := v.segmentSeq

	v.messageSeq++
	messageSequence := v.messageSeq

	turnID := v.currentTurnID

	v.mu.Unlock()

	speaker := models.SpeakerRoleAgent
	role := models.MessageRoleAssistant
	isLead := false

	if strings.EqualFold(
		strings.TrimSpace(ev.Role),
		"user",
	) {
		speaker = models.SpeakerRoleLead
		role = models.MessageRoleUser
		isLead = true
	}

	isVoicemail := isLead && isVoicemailSystemText(content)

	if isVoicemail {
		v.mu.Lock()
		v.voicemailDetected = true
		v.mu.Unlock()
	}

	language := languageFromEvent(
		ev,
		inferLanguageFromText(
			content,
			v.cfg.Language,
		),
	)

	if language != models.LanguageUnknown {
		if err := v.conv.UpdateCallPrimaryLanguage(
			ctx,
			v.cfg.CallID,
			language,
		); err != nil {
			return fmt.Errorf(
				"update primary language: %w",
				err,
			)
		}
	}

	detectedLanguages := append(
		[]string(nil),
		ev.Languages...,
	)

	if detectedLanguages == nil {
		detectedLanguages = []string{}
	}

	segmentID, err := v.conv.InsertTranscriptSegment(
		ctx,
		v.cfg.CallID,
		turnID,
		segmentSequence,
		speaker,
		content,
		language,
		detectedLanguages,
		nil,
		true,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf(
			"insert transcript segment: %w",
			err,
		)
	}

	if _, err := v.conv.InsertCallMessage(
		ctx,
		v.cfg.CallID,
		turnID,
		messageSequence,
		role,
		content,
	); err != nil {
		return fmt.Errorf(
			"insert call message: %w",
			err,
		)
	}

	if isLead &&
		v.functions != nil &&
		!isNonSubstantiveUserText(content) &&
		!isVoicemail {
		if err := v.functions.ProcessUserText(
			ctx,
			v.cfg.CallID,
			content,
			&segmentID,
		); err != nil {
			log.Printf(
				"voice_session: call=%s process user text: %v",
				v.cfg.CallID,
				err,
			)
		}
	}

	return nil
}

func (v *VoiceSession) handleFunctionCallRequest(
	ctx context.Context,
	ev AgentEvent,
) error {
	if v.functions == nil {
		return fmt.Errorf(
			"agent function executor is not configured",
		)
	}

	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf(
			"Deepgram connection is closed",
		)
	}

	for _, function := range ev.Functions {
		if !function.ClientSide {
			continue
		}

		v.respondToFunctionCall(ctx, conn, function)
	}

	return nil
}

func (v *VoiceSession) respondToFunctionCall(
	ctx context.Context,
	conn *AgentConn,
	function AgentFunctionCall,
) {
	result, err := v.functions.Execute(
		ctx,
		v.cfg.CallID,
		v.cfg.LeadID,
		function,
	)

	var payload []byte
	var marshalErr error

	if err != nil {
		log.Printf(
			"voice_session: call=%s function=%s execute error: %v",
			v.cfg.CallID,
			function.Name,
			err,
		)

		payload, marshalErr = json.Marshal(
			map[string]any{
				"success": false,
				"reason":  "temporary_failure",
			},
		)
	} else {
		payload, marshalErr = json.Marshal(
			map[string]any{
				"success": true,
				"result":  result,
			},
		)
	}

	if marshalErr != nil {
		log.Printf(
			"voice_session: call=%s function=%s marshal response: %v",
			v.cfg.CallID,
			function.Name,
			marshalErr,
		)

		return
	}

	if sendErr := conn.SendFunctionCallResponse(
		function.ID,
		function.Name,
		string(payload),
	); sendErr != nil {
		log.Printf(
			"voice_session: call=%s function=%s send response: %v",
			v.cfg.CallID,
			function.Name,
			sendErr,
		)
	}
}

func (v *VoiceSession) handleFunctionCallError(
	ev AgentEvent,
	err error,
) {
	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return
	}

	log.Printf(
		"voice_session: call=%s function call error: %v",
		v.cfg.CallID,
		err,
	)

	content, marshalErr := json.Marshal(
		map[string]any{
			"success": false,
			"reason":  "temporary_failure",
		},
	)
	if marshalErr != nil {
		log.Printf(
			"voice_session: call=%s marshal function error: %v",
			v.cfg.CallID,
			marshalErr,
		)

		return
	}

	for _, function := range ev.Functions {
		if !function.ClientSide {
			continue
		}

		if responseErr := conn.SendFunctionCallResponse(
			function.ID,
			function.Name,
			string(content),
		); responseErr != nil {
			log.Printf(
				"voice_session: call=%s function=%s response error: %v",
				v.cfg.CallID,
				function.Name,
				responseErr,
			)
		}
	}
}

func languageFromEvent(
	ev AgentEvent,
	fallback models.LanguageCode,
) models.LanguageCode {
	if len(ev.Languages) == 0 {
		return fallback
	}

	if len(ev.Languages) > 1 {
		return models.LanguageMixed
	}

	language := strings.ToLower(
		strings.TrimSpace(ev.Languages[0]),
	)

	if strings.Contains(language, "-") {
		language = strings.SplitN(
			language,
			"-",
			2,
		)[0]
	}

	switch language {
	case "en":
		return models.LanguageEnglish

	case "hi":
		return models.LanguageHindi

	case "te":
		return models.LanguageTelugu

	default:
		return fallback
	}
}

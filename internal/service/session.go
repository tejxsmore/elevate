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
	agentConn     *AgentConn
	inboundAudio  chan []byte
	outboundAudio chan []byte
	interruptions chan struct{}
	mu            sync.Mutex
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
	cfg VoiceSessionConfig,
) *VoiceSession {
	return &VoiceSession{
		cfg:           cfg,
		dgCfg:         dgCfg,
		deepgram:      NewDeepgramAgentClient(dgCfg),
		conv:          conv,
		functions:     functions,
		inboundAudio:  make(chan []byte, 256),
		outboundAudio: make(chan []byte, 256),
		interruptions: make(chan struct{}, 8),
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

func inferLanguageFromText(text string, fallback models.LanguageCode) models.LanguageCode {
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

func languageName(language models.LanguageCode) string {
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
	provider := strings.ToLower(strings.TrimSpace(v.dgCfg.SpeakProvider))

	if provider == "" {
		provider = "open_ai"
	}

	speed := v.dgCfg.SpeakSpeed

	if speed <= 0 {
		speed = 1.0
	}

	switch provider {
	case "open_ai":
		model := strings.TrimSpace(v.dgCfg.SpeakModel)

		if model == "" {
			model = "tts-1"
		}

		voice := strings.TrimSpace(v.dgCfg.SpeakVoice)

		if voice == "" {
			voice = "alloy"
		}

		return DeepgramProvider{
			Type:  "open_ai",
			Model: model,
			Voice: voice,
		}

	case "cartesia":
		return DeepgramProvider{
			Type:    "cartesia",
			ModelID: v.dgCfg.SpeakModelID,
			Voice: map[string]any{
				"mode": "id",
				"id":   v.dgCfg.SpeakVoice,
			},
			Language: "multi",
			Speed:    speed,
		}

	case "eleven_labs":
		return DeepgramProvider{
			Type:     "eleven_labs",
			ModelID:  v.dgCfg.SpeakModelID,
			Voice:    v.dgCfg.SpeakVoice,
			Language: "multi",
			Speed:    speed,
		}

	default:
		return DeepgramProvider{
			Type:    "deepgram",
			Model:   v.dgCfg.SpeakModel,
			Version: v.dgCfg.SpeakVersion,
			Speed:   speed,
		}
	}
}

func (v *VoiceSession) buildSpeakEndpoint(provider DeepgramProvider) *DeepgramEndpoint {
	if !strings.EqualFold(provider.Type, "open_ai") {
		return nil
	}

	key := strings.TrimSpace(v.dgCfg.OpenAIAPIKey)

	if key == "" {
		log.Printf(
			"voice_session: call=%s OPENAI_API_KEY is empty, speak endpoint will fail auth",
			v.cfg.CallID,
		)
		return nil
	}

	return &DeepgramEndpoint{
		URL: "https://api.openai.com/v1/audio/speech",
		Headers: map[string]string{
			"authorization": "Bearer " + key,
		},
	}
}

func (v *VoiceSession) buildSettings() DeepgramSettingsMessage {
	thinkProvider := strings.TrimSpace(v.cfg.ThinkProvider)

	if thinkProvider == "" {
		thinkProvider = strings.TrimSpace(v.dgCfg.ThinkProvider)
	}

	if thinkProvider == "" {
		thinkProvider = "open_ai"
	}

	thinkModel := strings.TrimSpace(v.cfg.ThinkModel)

	if thinkModel == "" {
		thinkModel = strings.TrimSpace(v.dgCfg.ThinkModel)
	}

	if thinkModel == "" {
		thinkModel = "gpt-5.6-luna"
	}

	listenModel := strings.TrimSpace(v.cfg.ListenModel)

	if listenModel == "" {
		listenModel = strings.TrimSpace(v.dgCfg.ListenModel)
	}

	if listenModel == "" {
		listenModel = "nova-3"
	}

	listenProvider := DeepgramProvider{
		Type:  "deepgram",
		Model: listenModel,
	}

	agentLanguage := strings.TrimSpace(v.dgCfg.ListenLanguage)

	if agentLanguage == "" {
		agentLanguage = "multi"
	}

	speakProvider := v.buildSpeakProvider()
	speakEndpoint := v.buildSpeakEndpoint(speakProvider)

	prompt := buildSalesPrompt(v.cfg.SystemPrompt)

	functions := []DeepgramFunction{}

	if v.functions != nil {
		functions = v.functions.Functions()
	}

	thinkProviderConfig := DeepgramProvider{
		Type:  thinkProvider,
		Model: thinkModel,
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

			Greeting: salesGreeting(),
		},

		Flags: &DeepgramFlags{
			History: true,
		},
	}
}

func (v *VoiceSession) Connect(ctx context.Context) error {
	if v == nil {
		return fmt.Errorf("voice_session: session is nil")
	}

	if v.deepgram == nil {
		return fmt.Errorf("voice_session: Deepgram client is not configured")
	}

	conn, err := v.deepgram.Connect(ctx, v.buildSettings())
	if err != nil {
		return fmt.Errorf("voice_session: connect Deepgram: %w", err)
	}

	v.mu.Lock()
	v.agentConn = conn
	v.mu.Unlock()

	return nil
}

func (v *VoiceSession) Loop(ctx context.Context) error {
	if v == nil {
		return fmt.Errorf("voice_session: session is nil")
	}

	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("voice_session: Loop called before Connect")
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
			"voice_session: call=%s connection lost (attempt %d/%d): %v",
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

		newConn, connectErr := v.deepgram.Connect(ctx, v.buildSettings())
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

func (v *VoiceSession) Run(ctx context.Context) error {
	if err := v.Connect(ctx); err != nil {
		return err
	}

	return v.Loop(ctx)
}

func (v *VoiceSession) pumpInboundAudio(ctx context.Context) {
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

			if err := conn.SendAudio(chunk); err != nil {
				log.Printf(
					"voice_session: call=%s send audio: %v",
					v.cfg.CallID,
					err,
				)
				continue
			}
		}
	}
}

func (v *VoiceSession) runOnce(ctx context.Context, conn *AgentConn) error {
	keepAliveDone := make(chan struct{})
	defer close(keepAliveDone)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
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

	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err, ok := <-conn.Errors():
			if !ok || err == nil {
				return nil
			}
			return err

		case event, ok := <-conn.Events():
			if !ok {
				select {
				case err := <-conn.Errors():
					if err != nil {
						return err
					}
				default:
				}
				return nil
			}

			if err := v.handleEvent(ctx, event); err != nil {
				log.Printf(
					"voice_session: call=%s event=%s: %v",
					v.cfg.CallID,
					event.Type,
					err,
				)

				if event.Type == EventFunctionCallRequest {
					v.handleFunctionCallError(event, err)
				}
			}
		}
	}
}

func (v *VoiceSession) handleEvent(ctx context.Context, ev AgentEvent) error {
	switch ev.Type {
	case EventAudioChunk:
		if len(ev.Audio) == 0 {
			return nil
		}

		select {
		case v.outboundAudio <- ev.Audio:
		case <-ctx.Done():
			return ctx.Err()
		}

	case EventUserStartedSpeaking:
		v.handleUserStartedSpeaking(ctx, ev)

	case EventConversationText:
		return v.persistConversationText(ctx, ev)

	case EventFunctionCallRequest:
		return v.handleFunctionCallRequest(ctx, ev)

	case EventAgentAudioDone:
		return v.handleAgentAudioDone(ctx)

	case EventWarning, EventError:
		log.Printf(
			"voice_session: call=%s deepgram %s: %s",
			v.cfg.CallID,
			ev.Type,
			string(ev.Raw),
		)
	}

	return nil
}

func (v *VoiceSession) handleUserStartedSpeaking(ctx context.Context, ev AgentEvent) {
	v.mu.Lock()
	v.turnSeq++
	sequence := v.turnSeq
	v.turnStartedAt = ev.ReceivedAt
	v.mu.Unlock()

	select {
	case v.interruptions <- struct{}{}:
	default:
	}

	turnID, err := v.conv.StartTurn(ctx, v.cfg.CallID, sequence)
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

func (v *VoiceSession) handleAgentAudioDone(ctx context.Context) error {
	v.mu.Lock()
	turnID := v.currentTurnID
	started := v.turnStartedAt
	sequence := v.turnSeq
	v.mu.Unlock()

	if turnID == nil {
		return nil
	}

	latency := int(time.Since(started).Milliseconds())

	if err := v.conv.CompleteTurn(ctx, *turnID, &latency); err != nil {
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

func (v *VoiceSession) persistConversationText(ctx context.Context, ev AgentEvent) error {
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

	if strings.EqualFold(strings.TrimSpace(ev.Role), "user") {
		speaker = models.SpeakerRoleLead
		role = models.MessageRoleUser
		isLead = true
	}

	language := languageFromEvent(
		ev,
		inferLanguageFromText(content, v.cfg.Language),
	)

	if isLead {
		inferredLanguage := inferLanguageFromText(content, v.cfg.Language)

		if inferredLanguage != models.LanguageUnknown {
			language = inferredLanguage
		}

		v.mu.Lock()
		conn := v.agentConn
		v.mu.Unlock()

		if conn != nil {
			updatedPrompt := buildSalesPrompt(v.cfg.SystemPrompt)

			updatedPrompt = strings.Join(
				[]string{
					updatedPrompt,
					"",
					"Current customer language:",
					languageName(language),
					"Match the customer's language for this response.",
				},
				"\n",
			)

			if err := conn.UpdatePrompt(updatedPrompt); err != nil {
				log.Printf(
					"voice_session: call=%s update language prompt: %v",
					v.cfg.CallID,
					err,
				)
			}
		}
	}

	if language != models.LanguageUnknown {
		if err := v.conv.UpdateCallPrimaryLanguage(ctx, v.cfg.CallID, language); err != nil {
			return fmt.Errorf("update primary language: %w", err)
		}
	}

	detectedLanguages := append([]string(nil), ev.Languages...)

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
		return fmt.Errorf("insert transcript segment: %w", err)
	}

	if _, err := v.conv.InsertCallMessage(
		ctx,
		v.cfg.CallID,
		turnID,
		messageSequence,
		role,
		content,
	); err != nil {
		return fmt.Errorf("insert call message: %w", err)
	}

	if isLead && v.functions != nil && !isNonSubstantiveUserText(content) {
		if err := v.functions.ProcessUserText(ctx, v.cfg.CallID, content, &segmentID); err != nil {
			log.Printf(
				"voice_session: call=%s process user text: %v",
				v.cfg.CallID,
				err,
			)
		}
	}

	return nil
}

func (v *VoiceSession) handleFunctionCallRequest(ctx context.Context, ev AgentEvent) error {
	if v.functions == nil {
		return fmt.Errorf("agent function executor is not configured")
	}

	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("Deepgram connection is closed")
	}

	for _, function := range ev.Functions {
		if !function.ClientSide {
			continue
		}

		result, err := v.functions.Execute(ctx, v.cfg.CallID, v.cfg.LeadID, function)

		if err != nil {
			errorResult, marshalErr := json.Marshal(
				map[string]any{
					"success": false,
					"error":   err.Error(),
				},
			)

			if marshalErr != nil {
				return marshalErr
			}

			if responseErr := conn.SendFunctionCallResponse(
				function.ID,
				function.Name,
				string(errorResult),
			); responseErr != nil {
				return responseErr
			}

			continue
		}

		successResult, marshalErr := json.Marshal(
			map[string]any{
				"success": true,
				"result":  result,
			},
		)

		if marshalErr != nil {
			return marshalErr
		}

		if err := conn.SendFunctionCallResponse(
			function.ID,
			function.Name,
			string(successResult),
		); err != nil {
			return err
		}
	}

	return nil
}

func (v *VoiceSession) handleFunctionCallError(ev AgentEvent, err error) {
	v.mu.Lock()
	conn := v.agentConn
	v.mu.Unlock()

	if conn == nil {
		return
	}

	content, marshalErr := json.Marshal(
		map[string]any{
			"success": false,
			"error":   err.Error(),
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

func languageFromEvent(ev AgentEvent, fallback models.LanguageCode) models.LanguageCode {
	if len(ev.Languages) == 0 {
		return fallback
	}

	if len(ev.Languages) > 1 {
		return models.LanguageMixed
	}

	language := strings.ToLower(strings.TrimSpace(ev.Languages[0]))

	if strings.Contains(language, "-") {
		language = strings.SplitN(language, "-", 2)[0]
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

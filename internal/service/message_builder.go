package service

import (
	"encoding/json"
	"strings"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type WhatsAppMessageBuilder struct{}

type WhatsAppFollowupContext struct {
	Lead           models.Lead
	Call           models.Call
	Discovery      models.DiscoveryProfile
	Messages       []repository.ConversationMessage
	Classification models.ClassificationLabel
	Campaign       models.Campaign
}

func NewWhatsAppMessageBuilder() *WhatsAppMessageBuilder {
	return &WhatsAppMessageBuilder{}
}

func (b *WhatsAppMessageBuilder) BuildFollowup(
	ctx WhatsAppFollowupContext,
) string {
	lines := []string{
		"Hi " + safeName(ctx.Lead.Name) + ",",
		"",
		"Thanks for speaking with me today. Here’s a quick recap of what we discussed:",
	}

	appendDiscoveryLine(
		&lines,
		"Business",
		ctx.Discovery.BusinessNiche,
	)

	appendDiscoveryLine(
		&lines,
		"Products",
		ctx.Discovery.ProductsSold,
	)

	appendDiscoveryLine(
		&lines,
		"Product count",
		ctx.Discovery.ProductCountEstimate,
	)

	appendDiscoveryLine(
		&lines,
		"Budget",
		ctx.Discovery.BudgetRange,
	)

	appendDiscoveryLine(
		&lines,
		"Timeline",
		ctx.Discovery.Timeline,
	)

	features := extractJSONFeatures(
		ctx.Discovery.FeaturesRequested,
	)

	if len(features) > 0 {
		lines = append(
			lines,
			"Features: "+strings.Join(
				features,
				", ",
			),
		)
	}

	if notes := optionalText(
		ctx.Discovery.ExtraNotes,
	); notes != "" {
		lines = append(
			lines,
			"Key point: "+notes,
		)
	}

	if ctx.Classification != "" &&
		ctx.Classification != models.ClassificationUnclassified {
		lines = append(
			lines,
			"Lead status: "+strings.ToUpper(
				string(ctx.Classification),
			),
		)
	}

	if summary := buildConversationSummary(
		ctx.Messages,
	); summary != "" {
		lines = append(
			lines,
			"",
			"What I captured from the conversation:",
			summary,
		)
	}

	lines = append(
		lines,
		"",
		buildNextStep(
			ctx.Discovery,
			ctx.Classification,
		),
	)

	if phone := campaignPhone(
		ctx.Campaign.AgentPhoneNumber,
	); phone != "" {
		lines = append(
			lines,
			"",
			"You can reach me at "+phone+".",
		)
	}

	lines = append(
		lines,
		"",
		"I’ve attached the architecture overview and my resume for reference.",
	)

	return strings.Join(
		lines,
		"\n",
	)
}

func (b *WhatsAppMessageBuilder) BuildBrochure(
	lead models.Lead,
	discovery models.DiscoveryProfile,
) string {
	lines := []string{
		"Hi " + safeName(lead.Name) + ",",
		"",
		"Thanks for speaking with me.",
		"Here’s the information we discussed:",
	}

	appendDiscoveryLine(
		&lines,
		"Business",
		discovery.BusinessNiche,
	)

	appendDiscoveryLine(
		&lines,
		"Products",
		discovery.ProductsSold,
	)

	appendDiscoveryLine(
		&lines,
		"Product count",
		discovery.ProductCountEstimate,
	)

	appendDiscoveryLine(
		&lines,
		"Budget",
		discovery.BudgetRange,
	)

	appendDiscoveryLine(
		&lines,
		"Timeline",
		discovery.Timeline,
	)

	features := extractJSONFeatures(
		discovery.FeaturesRequested,
	)

	if len(features) > 0 {
		lines = append(
			lines,
			"Features: "+strings.Join(
				features,
				", ",
			),
		)
	}

	lines = append(
		lines,
		"",
		"Feel free to reply here if you’d like to continue the discussion.",
	)

	return strings.Join(
		lines,
		"\n",
	)
}

func (b *WhatsAppMessageBuilder) BuildResume(
	lead models.Lead,
) string {
	return strings.Join(
		[]string{
			"Hi " + safeName(lead.Name) + ",",
			"",
			"Sharing my resume as discussed.",
			"",
			"Please feel free to reply here if you’d like to continue.",
		},
		"\n",
	)
}

func (b *WhatsAppMessageBuilder) BuildMidCall(
	lead models.Lead,
	discovery models.DiscoveryProfile,
	quote string,
) string {
	lines := []string{
		"Hi " + safeName(lead.Name) + ",",
		"",
		"Thanks. I’ve captured the details you shared.",
	}

	appendDiscoveryLine(
		&lines,
		"Business",
		discovery.BusinessNiche,
	)

	appendDiscoveryLine(
		&lines,
		"Products",
		discovery.ProductsSold,
	)

	appendDiscoveryLine(
		&lines,
		"Product count",
		discovery.ProductCountEstimate,
	)

	appendDiscoveryLine(
		&lines,
		"Budget",
		discovery.BudgetRange,
	)

	appendDiscoveryLine(
		&lines,
		"Timeline",
		discovery.Timeline,
	)

	features := extractJSONFeatures(
		discovery.FeaturesRequested,
	)

	if len(features) > 0 {
		lines = append(
			lines,
			"Features: "+strings.Join(
				features,
				", ",
			),
		)
	}

	if value := strings.TrimSpace(
		quote,
	); value != "" {
		lines = append(
			lines,
			"",
			`You mentioned: "`+value+`"`,
		)
	}

	return strings.Join(
		lines,
		"\n",
	)
}

func safeName(
	name *string,
) string {
	if name == nil {
		return "there"
	}

	value := strings.TrimSpace(*name)

	if value == "" {
		return "there"
	}

	return value
}

func optionalText(
	value *string,
) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func campaignPhone(
	value *string,
) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func appendDiscoveryLine(
	lines *[]string,
	label string,
	value *string,
) {
	valueText := optionalText(value)

	if valueText == "" {
		return
	}

	*lines = append(
		*lines,
		label+": "+valueText,
	)
}

func extractJSONFeatures(
	raw models.JSONB,
) []string {
	if len(raw) == 0 {
		return nil
	}

	var values []string

	if err := json.Unmarshal(
		raw,
		&values,
	); err != nil {
		return nil
	}

	result := make(
		[]string,
		0,
		len(values),
	)

	seen := map[string]struct{}{}

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		key := strings.ToLower(value)

		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}

		result = append(
			result,
			value,
		)
	}

	return result
}

func buildConversationSummary(
	messages []repository.ConversationMessage,
) string {
	leadMessages := make(
		[]string,
		0,
	)

	for _, message := range messages {
		if message.Role != models.MessageRoleUser {
			continue
		}

		content := strings.TrimSpace(
			message.Content,
		)

		if content == "" {
			continue
		}

		leadMessages = append(
			leadMessages,
			content,
		)
	}

	if len(leadMessages) == 0 {
		return ""
	}

	const maxMessages = 3

	start := 0

	if len(leadMessages) > maxMessages {
		start = len(leadMessages) - maxMessages
	}

	selected := leadMessages[start:]

	lines := make(
		[]string,
		0,
		len(selected),
	)

	for _, message := range selected {
		lines = append(
			lines,
			"• "+message,
		)
	}

	return strings.Join(
		lines,
		"\n",
	)
}

func buildNextStep(
	discovery models.DiscoveryProfile,
	classification models.ClassificationLabel,
) string {
	switch classification {
	case models.ClassificationHot:
		return "Based on what you shared, the next step would be to discuss the implementation and get the project moving."

	case models.ClassificationWarm:
		if optionalText(discovery.Timeline) != "" {
			return "Based on your timeline, we can use this as the starting point for the next discussion."
		}

		return "We can use these details as the starting point for the next discussion."

	case models.ClassificationCold:
		return "Whenever you’re ready, feel free to reply here and we can continue the discussion."

	default:
		return "We’ll use these details as the starting point for the next step."
	}
}

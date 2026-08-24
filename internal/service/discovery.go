package service

import (
	"regexp"
	"strconv"
	"strings"

	"elevate/internal/models"
)

type DiscoveryExtraction struct {
	BusinessNiche        string
	ProductsSold         string
	ProductCountEstimate string
	BudgetRange          string
	BudgetRawText        string
	Timeline             string
	TimelineRawText      string
	FeaturesRequested    []string
	BarrierType          models.BarrierType
	BarrierDetail        string
	HasBarrier           bool
	CallbackRequest      string
}

type DiscoveryExtractor struct {
	budgetPatterns []*regexp.Regexp
}

func NewDiscoveryExtractor() *DiscoveryExtractor {
	return &DiscoveryExtractor{
		budgetPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:budget|spend|invest|around|upto|up to)\s*(?:is|of|around)?\s*[₹rs\. ]*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)?`),
			regexp.MustCompile(`(?i)[₹rs\. ]*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)\b`),
			regexp.MustCompile(`(?i)₹\s*([0-9][0-9,]*(?:\.[0-9]+)?)`),
		},
	}
}

func (e *DiscoveryExtractor) Extract(text string) DiscoveryExtraction {
	lower := strings.ToLower(strings.TrimSpace(text))

	out := DiscoveryExtraction{
		FeaturesRequested: make([]string, 0),
	}

	out.BusinessNiche = extractBusinessNiche(lower)
	out.ProductsSold = extractProducts(lower)
	out.ProductCountEstimate = extractProductCount(lower)

	if budget, raw := e.extractBudget(text); budget != "" {
		out.BudgetRange = budget
		out.BudgetRawText = raw
	}

	out.Timeline = extractTimeline(lower)
	out.TimelineRawText = extractTimelineRaw(text)
	out.FeaturesRequested = extractFeatures(lower)

	if barrier, detail := extractBarrier(lower); barrier != "" {
		out.HasBarrier = true
		out.BarrierType = models.BarrierType(barrier)
		out.BarrierDetail = detail
	}

	out.CallbackRequest = extractCallbackRequest(lower)

	return out
}

func extractBusinessNiche(text string) string {
	patterns := []string{
		`(?i)i (?:sell|run|have|own) (.+?)(?:business|store|shop)`,
		`(?i)(?:my|our) business is (.+)`,
		`(?i)(?:i am|i'm) in (.+)`,
		`(?i)i sell (.+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 1 {
			return cleanValue(match[1])
		}
	}

	niches := []string{
		"clothing",
		"fashion",
		"jewellery",
		"jewelry",
		"electronics",
		"grocery",
		"food",
		"cosmetics",
		"beauty",
		"furniture",
		"books",
		"footwear",
		"retail",
	}

	for _, niche := range niches {
		if strings.Contains(text, niche) {
			return niche
		}
	}

	return ""
}

func extractProducts(text string) string {
	patterns := []string{
		`(?i)i sell (.+?)(?:online|on my website|through instagram|through whatsapp|$)`,
		`(?i)(?:products|items) (?:are|include) (.+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 1 {
			return cleanValue(match[1])
		}
	}

	return ""
}

func extractProductCount(text string) string {
	patterns := []string{
		`(?i)([0-9][0-9,]*)\s*(?:products|items|skus|sku)`,
		`(?i)(?:around|about|roughly)\s*([0-9][0-9,]*)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

func (e *DiscoveryExtractor) extractBudget(text string) (string, string) {
	for _, pattern := range e.budgetPatterns {
		match := pattern.FindStringSubmatch(text)

		if len(match) < 2 {
			continue
		}

		rawNumber := strings.ReplaceAll(
			match[1],
			",",
			"",
		)

		value, err := strconv.ParseFloat(
			rawNumber,
			64,
		)
		if err != nil {
			continue
		}

		if len(match) >= 3 {
			switch strings.ToLower(match[2]) {
			case "k", "thousand":
				value *= 1000
			case "lakh", "lakhs", "lac", "lacs":
				value *= 100000
			}
		}

		return "₹" + strconv.FormatInt(
			int64(value),
			10,
		), match[0]
	}

	return "", ""
}

func extractTimeline(text string) string {
	patterns := map[string]string{
		"tomorrow":       "tomorrow",
		"next week":      "next week",
		"next month":     "next month",
		"this week":      "this week",
		"this month":     "this month",
		"two months":     "2 months",
		"2 months":       "2 months",
		"three months":   "3 months",
		"3 months":       "3 months",
		"one month":      "1 month",
		"1 month":        "1 month",
		"within a month": "within 1 month",
		"दो महीने":       "2 months",
		"अगले महीने":     "next month",
		"अगले हफ्ते":     "next week",
		"दो महीने में":   "within 2 months",
		"రెండు నెలల్లో":  "within 2 months",
		"వచ్చే నెల":      "next month",
		"వచ్చే వారం":     "next week",
	}

	for key, value := range patterns {
		if strings.Contains(text, key) {
			return value
		}
	}

	re := regexp.MustCompile(
		`(?i)(?:within|in)\s+([0-9]+)\s+(day|days|week|weeks|month|months)`,
	)

	if match := re.FindStringSubmatch(text); len(match) > 2 {
		return match[1] + " " + match[2]
	}

	return ""
}

func extractTimelineRaw(text string) string {
	re := regexp.MustCompile(
		`(?i)(?:within|in|by|before|after|tomorrow|next)\s+[^,.!?]+`,
	)

	if match := re.FindString(text); match != "" {
		return strings.TrimSpace(match)
	}

	return ""
}

func extractFeatures(text string) []string {
	features := map[string]string{
		"payment":        "payments",
		"payments":       "payments",
		"razorpay":       "payments",
		"upi":            "payments",
		"inventory":      "inventory",
		"stock":          "inventory",
		"whatsapp":       "whatsapp",
		"chat":           "chat",
		"login":          "user accounts",
		"accounts":       "user accounts",
		"authentication": "authentication",
		"search":         "search",
		"filter":         "filters",
		"filters":        "filters",
		"delivery":       "delivery",
		"shipping":       "shipping",
		"admin panel":    "admin panel",
		"dashboard":      "dashboard",
		"analytics":      "analytics",
		"coupon":         "coupons",
		"discount":       "discounts",
		"reviews":        "reviews",
		"wishlist":       "wishlist",
	}

	seen := map[string]struct{}{}
	result := make([]string, 0)

	for phrase, normalized := range features {
		if !strings.Contains(text, phrase) {
			continue
		}

		if _, exists := seen[normalized]; exists {
			continue
		}

		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	return result
}

func extractBarrier(text string) (string, string) {
	patterns := []struct {
		kind    string
		phrases []string
	}{
		{
			kind: "budget",
			phrases: []string{
				"budget is low",
				"budget is tight",
				"too expensive",
				"can't afford",
				"cannot afford",
				"not much budget",
				"budget problem",
				"budget कम",
				"बजट कम",
				"బడ్జెట్ తక్కువ",
				"బడ్జెట్ లేదు",
			},
		},
		{
			kind: "timing",
			phrases: []string{
				"not now",
				"later",
				"next month",
				"next year",
				"not ready",
				"need some time",
				"अभी नहीं",
				"बाद में",
				"ఇప్పుడు కాదు",
				"తర్వాత",
			},
		},
		{
			kind: "decision_maker",
			phrases: []string{
				"my brother handles",
				"my husband handles",
				"my wife handles",
				"my partner handles",
				"my father decides",
				"someone else decides",
				"need to ask",
				"need to discuss",
				"decision maker",
				"भाई से पूछ",
				"पति से पूछ",
				"पत्नी से पूछ",
				"मेरे भाई",
				"मेरे पति",
				"मेरी पत्नी",
				"బ్రదర్ తో",
				"అన్నతో మాట్లాడాలి",
			},
		},
		{
			kind: "trust",
			phrases: []string{
				"not sure",
				"don't trust",
				"need to see examples",
				"show me previous work",
				"how do i know",
				"यकीन नहीं",
				"पहले काम दिखाओ",
				"నమ్మకం లేదు",
				"ముందు పని చూపించండి",
			},
		},
	}

	for _, pattern := range patterns {
		for _, phrase := range pattern.phrases {
			if strings.Contains(text, phrase) {
				return pattern.kind, phrase
			}
		}
	}

	return "", ""
}

func extractCallbackRequest(text string) string {
	patterns := []string{
		"call me back",
		"call back",
		"call tomorrow",
		"call next week",
		"call later",
		"contact me tomorrow",
		"contact me later",
		"call me in",
		"मुझे कल कॉल करना",
		"कल कॉल करना",
		"बाद में कॉल करना",
		"फिर कॉल करना",
		"రేపు కాల్ చేయండి",
		"తర్వాత కాల్ చేయండి",
		"మళ్లీ కాల్ చేయండి",
	}

	for _, phrase := range patterns {
		if strings.Contains(text, phrase) {
			return phrase
		}
	}

	return ""
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".,!?")
	return value
}

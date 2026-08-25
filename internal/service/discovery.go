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
			regexp.MustCompile(`(?i)(?:budget|spend|invest)\s*(?:is|of|around|about|roughly)?\s*[₹rs\. ]*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)?`),
			regexp.MustCompile(`(?i)(?:around|about|roughly|upto|up to)\s*[₹rs\. ]*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)\b`),
			regexp.MustCompile(`(?i)₹\s*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)?`),
			regexp.MustCompile(`(?i)\b([0-9][0-9,]*(?:\.[0-9]+)?)\s*(k|thousand|lakh|lakhs|lac|lacs)\b`),
		},
	}
}

func (e *DiscoveryExtractor) Extract(text string) DiscoveryExtraction {
	original := strings.TrimSpace(text)
	lower := strings.ToLower(original)

	out := DiscoveryExtraction{
		FeaturesRequested: make([]string, 0),
	}

	out.BusinessNiche = extractBusinessNiche(lower)
	out.ProductsSold = extractProducts(lower)
	out.ProductCountEstimate = extractProductCount(lower)

	if budget, raw := e.extractBudget(original); budget != "" {
		out.BudgetRange = budget
		out.BudgetRawText = raw
	}

	out.Timeline, out.TimelineRawText =
		extractTimeline(original)

	out.FeaturesRequested =
		extractFeatures(lower)

	if barrier, detail := extractBarrier(lower); barrier != "" {
		out.HasBarrier = true
		out.BarrierType = models.BarrierType(barrier)
		out.BarrierDetail = detail
	}

	out.CallbackRequest =
		extractCallbackRequest(lower)

	return out
}

func extractBusinessNiche(text string) string {
	patterns := []string{
		`(?i)i\s+(?:sell|run|have|own)\s+(.+?)\s+(?:business|store|shop|cafe|restaurant)`,
		`(?i)(?:my|our)\s+business\s+is\s+(.+)`,
		`(?i)(?:i\s+am|i'm)\s+in\s+(.+)`,
		`(?i)i\s+sell\s+(.+)`,
		`(?i)i\s+run\s+a\s+(.+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 1 {
			value := cleanValue(match[1])

			value = strings.TrimSpace(
				strings.TrimSuffix(value, "business"),
			)
			value = strings.TrimSpace(
				strings.TrimSuffix(value, "store"),
			)
			value = strings.TrimSpace(
				strings.TrimSuffix(value, "shop"),
			)

			if value != "" {
				return value
			}
		}
	}

	niches := []string{
		"cafe",
		"cafes",
		"restaurant",
		"restaurants",
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
		"bakery",
		"salon",
		"pharmacy",
		"handicrafts",
		"home decor",
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
		`(?i)i\s+sell\s+(.+?)(?:\s+online|\s+on\s+my\s+website|\s+through\s+instagram|\s+through\s+whatsapp|$)`,
		`(?i)(?:i\s+sell|we\s+sell|our\s+products\s+are|products\s+include|items\s+include)\s+(.+)`,
		`(?i)(?:we\s+have|we\s+offer)\s+(.+?)(?:\s+online|\s+on\s+our\s+website|$)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 1 {
			value := cleanValue(match[1])

			if value != "" {
				return value
			}
		}
	}

	return ""
}

func extractProductCount(text string) string {
	patterns := []string{
		`(?i)\b([0-9][0-9,]*)\s*(?:products|items|skus|sku)\b`,
		`(?i)\b(?:around|about|roughly|approximately)\s+([0-9][0-9,]*)\s*(?:products|items|skus|sku)?\b`,
		`(?i)\b(?:around|about|roughly|approximately)\s+(a\s+)?hundred\b`,
		`(?i)\b(?:around|about|roughly|approximately)\s+(a\s+)?thousand\b`,
		`(?i)\b(?:targeting|planning|have)\s+(?:about|around|roughly)?\s*([0-9][0-9,]*)\b`,
		`(?i)\b(?:targeting|planning|have)\s+(?:about|around|roughly)?\s+(?:a\s+)?hundred\b`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)

		if match := re.FindStringSubmatch(text); len(match) > 0 {
			full := strings.ToLower(
				strings.TrimSpace(match[0]),
			)

			if strings.Contains(full, "hundred") {
				return "100"
			}

			if strings.Contains(full, "thousand") {
				return "1000"
			}

			for i := len(match) - 1; i >= 1; i-- {
				if match[i] != "" {
					return strings.TrimSpace(match[i])
				}
			}
		}
	}

	return ""
}

func (e *DiscoveryExtractor) extractBudget(
	text string,
) (string, string) {
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
			switch strings.ToLower(
				strings.TrimSpace(match[2]),
			) {
			case "k", "thousand":
				value *= 1000

			case "lakh", "lakhs", "lac", "lacs":
				value *= 100000
			}
		}

		return "₹" + strconv.FormatInt(
			int64(value),
			10,
		), strings.TrimSpace(match[0])
	}

	wordBudgets := []struct {
		pattern string
		value   string
	}{
		{
			`(?i)\b(?:around|about|roughly)?\s*fifty\s+thousand\b`,
			"₹50000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*sixty\s+thousand\b`,
			"₹60000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*seventy\s+thousand\b`,
			"₹70000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*eighty\s+thousand\b`,
			"₹80000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*ninety\s+thousand\b`,
			"₹90000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*one\s+lakh\b`,
			"₹100000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*one\s+lac\b`,
			"₹100000",
		},
		{
			`(?i)\b(?:around|about|roughly)?\s*two\s+lakh\b`,
			"₹200000",
		},
	}

	for _, item := range wordBudgets {
		re := regexp.MustCompile(item.pattern)

		if match := re.FindString(text); match != "" {
			return item.value, strings.TrimSpace(match)
		}
	}

	return "", ""
}

func extractTimeline(
	text string,
) (string, string) {
	patterns := []struct {
		re   *regexp.Regexp
		name string
		raw  func(string) string
	}{
		{
			re: regexp.MustCompile(
				`(?i)\bwithin\s+([0-9]+)\s+(day|days|week|weeks|month|months)\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bin\s+([0-9]+)\s+(day|days|week|weeks|month|months)\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bwithin\s+a\s+month\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bwithin\s+one\s+month\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bnext\s+(week|month|year)\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bthis\s+(week|month)\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\btomorrow\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bone\s+month\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\btwo\s+months?\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bthree\s+months?\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bअगले\s+महीने\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bअगले\s+हफ्ते\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bदो\s+महीने\s+में\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bविचित\s+महीने\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bరెండు\s+నెలల్లో\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bవచ్చే\s+నెల\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
		{
			re: regexp.MustCompile(
				`(?i)\bవచ్చే\s+వారం\b`,
			),
			raw: func(value string) string {
				return value
			},
		},
	}

	for _, pattern := range patterns {
		match := pattern.re.FindString(text)

		if match == "" {
			continue
		}

		raw := cleanTimelineRaw(
			pattern.raw(
				strings.TrimSpace(match),
			),
		)

		return normalizeTimeline(raw), raw
	}

	return "", ""
}

func normalizeTimeline(
	raw string,
) string {
	text := strings.ToLower(
		strings.TrimSpace(raw),
	)

	switch text {
	case "tomorrow":
		return "tomorrow"

	case "next week":
		return "next week"

	case "next month":
		return "next month"

	case "next year":
		return "next year"

	case "this week":
		return "this week"

	case "this month":
		return "this month"

	case "within a month",
		"within one month",
		"one month":
		return "within 1 month"

	case "two months",
		"two months in":
		return "2 months"

	case "three months":
		return "3 months"
	}

	re := regexp.MustCompile(
		`(?i)(?:within|in)\s+([0-9]+)\s+(day|days|week|weeks|month|months)`,
	)

	if match := re.FindStringSubmatch(text); len(match) > 2 {
		unit := strings.ToLower(match[2])

		switch unit {
		case "day":
			unit = "day"

		case "days":
			unit = "days"

		case "week":
			unit = "week"

		case "weeks":
			unit = "weeks"

		case "month":
			unit = "month"

		case "months":
			unit = "months"
		}

		return match[1] + " " + unit
	}

	switch {
	case strings.Contains(text, "अगले महीने"):
		return "next month"

	case strings.Contains(text, "अगले हफ्ते"):
		return "next week"

	case strings.Contains(text, "दो महीने"):
		return "2 months"

	case strings.Contains(text, "వచ్చే నెల"):
		return "next month"

	case strings.Contains(text, "వచ్చే వారం"):
		return "next week"

	case strings.Contains(text, "రెండు నెలల్లో"):
		return "within 2 months"
	}

	return raw
}

func cleanTimelineRaw(
	value string,
) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ".,!?;:")

	return value
}

func extractFeatures(text string) []string {
	type featureRule struct {
		normalized string
		patterns   []string
	}

	rules := []featureRule{
		{
			normalized: "payments",
			patterns: []string{
				"payment gateway",
				"payment gateways",
				"online payments",
				"online payment",
				"razorpay",
				"upi payments",
				"upi integration",
				"payment integration",
				"easy payment",
				"easy payments",
			},
		},
		{
			normalized: "inventory",
			patterns: []string{
				"inventory management",
				"inventory",
				"stock management",
				"stock tracking",
			},
		},
		{
			normalized: "whatsapp",
			patterns: []string{
				"whatsapp integration",
				"whatsapp button",
				"whatsapp chat",
				"whatsapp support",
				"whatsapp notifications",
			},
		},
		{
			normalized: "chat",
			patterns: []string{
				"live chat",
				"customer chat",
				"chat support",
			},
		},
		{
			normalized: "user accounts",
			patterns: []string{
				"user login",
				"user accounts",
				"customer accounts",
				"customer login",
			},
		},
		{
			normalized: "authentication",
			patterns: []string{
				"authentication",
				"sign in",
				"sign up",
				"login system",
			},
		},
		{
			normalized: "search",
			patterns: []string{
				"product search",
				"search products",
				"search bar",
			},
		},
		{
			normalized: "filters",
			patterns: []string{
				"filters",
				"filter products",
				"product filters",
			},
		},
		{
			normalized: "shipping",
			patterns: []string{
				"shipping integration",
				"shipping options",
				"shipping calculation",
				"shipping provider",
			},
		},
		{
			normalized: "delivery",
			patterns: []string{
				"delivery integration",
				"delivery tracking",
				"delivery scheduling",
				"delivery management",
				"delivery option",
				"delivery options",
			},
		},
		{
			normalized: "admin panel",
			patterns: []string{
				"admin panel",
				"admin dashboard",
			},
		},
		{
			normalized: "dashboard",
			patterns: []string{
				"dashboard",
			},
		},
		{
			normalized: "analytics",
			patterns: []string{
				"analytics",
				"sales analytics",
				"website analytics",
			},
		},
		{
			normalized: "coupons",
			patterns: []string{
				"coupon system",
				"coupon codes",
				"coupons",
			},
		},
		{
			normalized: "discounts",
			patterns: []string{
				"discount system",
				"discounts",
				"discount codes",
			},
		},
		{
			normalized: "reviews",
			patterns: []string{
				"customer reviews",
				"product reviews",
				"reviews",
			},
		},
		{
			normalized: "wishlist",
			patterns: []string{
				"wishlist",
				"wish list",
			},
		},
		{
			normalized: "membership",
			patterns: []string{
				"membership",
				"membership rewards",
				"loyalty program",
				"loyalty rewards",
			},
		},
	}

	seen := make(
		map[string]struct{},
	)

	result := make(
		[]string,
		0,
	)

	for _, rule := range rules {
		for _, pattern := range rule.patterns {
			if !strings.Contains(
				text,
				pattern,
			) {
				continue
			}

			if featureMentionIsNegativeContext(
				text,
				pattern,
			) {
				continue
			}

			if _, exists := seen[rule.normalized]; exists {
				break
			}

			seen[rule.normalized] = struct{}{}

			result = append(
				result,
				rule.normalized,
			)

			break
		}
	}

	return result
}

func featureMentionIsNegativeContext(
	text string,
	phrase string,
) bool {
	index := strings.Index(
		text,
		phrase,
	)

	if index < 0 {
		return false
	}

	start := index - 80

	if start < 0 {
		start = 0
	}

	context := text[start:index]

	negativePatterns := []string{
		"handled by",
		"taken care by",
		"taken care of by",
		"managed by",
		"provided by",
		"through swiggy",
		"through zomato",
		"via swiggy",
		"via zomato",
		"don't need",
		"do not need",
		"not needed",
		"not required",
		"already handled",
	}

	for _, negative := range negativePatterns {
		if strings.Contains(
			context,
			negative,
		) {
			return true
		}
	}

	if phrase == "whatsapp" {
		actionContext := []string{
			"send me",
			"send it",
			"send details",
			"send the details",
			"send via",
			"send on",
			"contact me on",
			"message me on",
		}

		for _, marker := range actionContext {
			if strings.Contains(
				context,
				marker,
			) {
				return true
			}
		}
	}

	return false
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
			if strings.Contains(
				text,
				phrase,
			) {
				return pattern.kind, phrase
			}
		}
	}

	return "", ""
}

func extractCallbackRequest(
	text string,
) string {
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
		"रేపు కాల్ చేయండి",
		"తర్వాత కాల్ చేయండి",
		"మళ్లీ కాల్ చేయండి",
	}

	for _, phrase := range patterns {
		if strings.Contains(
			text,
			phrase,
		) {
			return phrase
		}
	}

	return ""
}

func cleanValue(
	value string,
) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(
		value,
		".!?",
	)

	return value
}

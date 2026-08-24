package service

import (
	"strings"

	"elevate/internal/models"
)

type ClassificationResult struct {
	Label      models.ClassificationLabel
	Confidence float64
	Summary    string
	Signals    map[string]any
}

type Classifier struct{}

func NewClassifier() *Classifier {
	return &Classifier{}
}

func (c *Classifier) Classify(
	text string,
	profile models.DiscoveryProfile,
	hasBarrier bool,
	barrier models.BarrierType,
) ClassificationResult {
	lower := strings.ToLower(strings.TrimSpace(text))

	signals := map[string]any{
		"budget_known": profile.BudgetRange != nil &&
			strings.TrimSpace(*profile.BudgetRange) != "",
		"timeline_known": profile.Timeline != nil &&
			strings.TrimSpace(*profile.Timeline) != "",
		"features_known":  len(profile.FeaturesRequested) >= 2,
		"barrier_present": hasBarrier,
	}

	hotSignals := 0
	warmSignals := 0

	hotPhrases := []string{
		"i want to start",
		"want to start",
		"need it",
		"i need",
		"let's do it",
		"lets do it",
		"how soon can you start",
		"when can you start",
		"what is the price",
		"how much",
		"send me the details",
		"send details",
		"send it on whatsapp",
		"send on whatsapp",
		"can we start",
		"i want the website",
		"need a website",
		"ready to start",
		"book it",
		"let's proceed",
		"proceed",
		"शुरू करना",
		"शुरू कर सकते",
		"व्हाट्सऐप पर भेज",
		"व्हाट्सएप पर भेज",
		"రేపటి",
	}

	for _, phrase := range hotPhrases {
		if strings.Contains(lower, phrase) {
			hotSignals++
			signals["intent_"+phrase] = true
		}
	}

	if signals["budget_known"] == true {
		hotSignals++
	}

	if signals["timeline_known"] == true {
		hotSignals++
	}

	if signals["features_known"] == true {
		hotSignals++
	}

	if hasBarrier {
		warmSignals += 2
		signals["barrier_type"] = barrier
	}

	warmPhrases := []string{
		"i'll think",
		"i will think",
		"let me think",
		"later",
		"not now",
		"maybe",
		"need to discuss",
		"need to ask",
		"my brother",
		"my husband",
		"my wife",
		"my partner",
		"budget is tight",
		"budget is low",
		"not ready",
		"discuss with",
		"बाद में",
		"सोचकर",
		"सोच के",
		"भाई से पूछ",
		"पति से पूछ",
		"पत्नी से पूछ",
		"बजट कम",
		"अभी नहीं",
		"తర్వాత",
		"ఆలోచించి",
		"బ్రదర్ తో",
		"బడ్జెట్ లేదు",
		"ఇప్పుడు కాదు",
	}

	for _, phrase := range warmPhrases {
		if strings.Contains(lower, phrase) {
			warmSignals++
			signals["barrier_"+phrase] = true
		}
	}

	coldPhrases := []string{
		"just looking",
		"just checking",
		"just exploring",
		"only browsing",
		"no plans",
		"just curious",
		"not interested",
		"बस देख रहा",
		"सिर्फ देख",
		"अभी कोई प्लान नहीं",
		"बस पूछ रहा",
		"చూస్తున్నా",
		"ఇంకా ప్లాన్ లేదు",
	}

	for _, phrase := range coldPhrases {
		if strings.Contains(lower, phrase) {
			return ClassificationResult{
				Label:      models.ClassificationCold,
				Confidence: 0.88,
				Summary:    "Lead is currently browsing or does not show a concrete buying need.",
				Signals:    signals,
			}
		}
	}

	if hotSignals >= 3 && !hasBarrier {
		return ClassificationResult{
			Label:      models.ClassificationHot,
			Confidence: confidenceForHot(hotSignals),
			Summary:    "Lead shows concrete buying intent with commercial discovery signals.",
			Signals:    signals,
		}
	}

	if warmSignals > 0 || hasBarrier {
		return ClassificationResult{
			Label:      models.ClassificationWarm,
			Confidence: confidenceForWarm(warmSignals),
			Summary:    "Lead is interested but has a timing, budget, decision-maker, or trust barrier.",
			Signals:    signals,
		}
	}

	if hotSignals >= 2 {
		return ClassificationResult{
			Label:      models.ClassificationWarm,
			Confidence: 0.72,
			Summary:    "Lead shows interest but needs further qualification before being treated as hot.",
			Signals:    signals,
		}
	}

	return ClassificationResult{
		Label:      models.ClassificationCold,
		Confidence: 0.62,
		Summary:    "Lead has not shown enough concrete buying intent yet.",
		Signals:    signals,
	}
}

func confidenceForHot(signals int) float64 {
	if signals >= 6 {
		return 0.96
	}

	if signals >= 5 {
		return 0.92
	}

	return 0.87
}

func confidenceForWarm(signals int) float64 {
	if signals >= 4 {
		return 0.91
	}

	if signals >= 2 {
		return 0.84
	}

	return 0.76
}

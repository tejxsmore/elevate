package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"elevate/internal/models"
	"elevate/internal/repository"
)

type CallbackResolution struct {
	ScheduledFor      *time.Time
	Timezone          string
	Confidence        float64
	ResolutionSource  string
	ResolvedFrom      map[string]any
	NeedsConfirmation bool
}

type CallbackService struct {
	repo *repository.CallbackRepo
}

func NewCallbackService(
	r *repository.CallbackRepo,
) *CallbackService {
	return &CallbackService{
		repo: r,
	}
}

func (s *CallbackService) Resolve(
	text string,
	now time.Time,
	timezone string,
) CallbackResolution {
	original := strings.TrimSpace(text)
	lower := strings.ToLower(original)

	location := resolveTimezone(timezone)

	now = now.In(location)

	result := CallbackResolution{
		Timezone: location.String(),
		ResolvedFrom: map[string]any{
			"original_text": original,
			"timezone":      location.String(),
		},
	}

	if !hasCallbackIntent(lower) {
		result.NeedsConfirmation = true
		result.Confidence = 0
		result.ResolutionSource = "no_callback_intent"

		return result
	}

	base,
		dateConfidence,
		dateSource,
		dateFound := resolveDate(
		lower,
		now,
	)

	if !dateFound {
		result.NeedsConfirmation = true
		result.Confidence = 0.52
		result.ResolutionSource = "ambiguous_date"
		result.ResolvedFrom["date_source"] = "ambiguous"

		return result
	}

	result.ResolvedFrom["date_source"] =
		dateSource

	hour,
		minute,
		timeConfidence,
		timeSource,
		timeFound := resolveTime(
		lower,
	)

	if !timeFound {
		result.NeedsConfirmation = true
		result.Confidence = dateConfidence
		result.ResolutionSource = dateSource
		result.ResolvedFrom["time_source"] = "ambiguous"

		return result
	}

	base = time.Date(
		base.Year(),
		base.Month(),
		base.Day(),
		hour,
		minute,
		0,
		0,
		location,
	)

	if !base.After(now) {
		switch dateSource {
		case "tomorrow",
			"day_after_tomorrow",
			"next_week",
			"next_month":
			// Explicitly future relative dates remain unchanged.

		default:
			base = base.AddDate(
				0,
				0,
				1,
			)

			result.ResolvedFrom["rolled_forward"] = true
		}
	}

	confidence :=
		dateConfidence * timeConfidence

	result.ScheduledFor = &base
	result.Confidence = confidence
	result.ResolutionSource =
		dateSource + "+" + timeSource

	result.ResolvedFrom["resolved_local_time"] =
		base.Format(time.RFC3339)

	result.NeedsConfirmation =
		confidence < 0.78

	result.ResolvedFrom["needs_confirmation"] =
		result.NeedsConfirmation

	return result
}

func (s *CallbackService) Create(
	ctx context.Context,
	callID uuid.UUID,
	leadID uuid.UUID,
	requestedText string,
	resolution CallbackResolution,
	actionID *uuid.UUID,
) (models.ScheduledCallback, error) {
	if s == nil || s.repo == nil {
		return models.ScheduledCallback{}, fmt.Errorf(
			"callback service is not configured",
		)
	}

	status :=
		models.CallbackNeedsConfirmation

	if resolution.ScheduledFor != nil &&
		!resolution.NeedsConfirmation {
		status = models.CallbackScheduled
	}

	if strings.TrimSpace(
		resolution.Timezone,
	) == "" {
		resolution.Timezone =
			"Asia/Kolkata"
	}

	if resolution.ResolvedFrom == nil {
		resolution.ResolvedFrom =
			map[string]any{}
	}

	resolution.ResolvedFrom["confidence"] =
		resolution.Confidence

	resolution.ResolvedFrom["resolution_source"] =
		resolution.ResolutionSource

	resolution.ResolvedFrom["needs_confirmation"] =
		resolution.NeedsConfirmation

	resolution.ResolvedFrom["timezone"] =
		resolution.Timezone

	resolvedFrom, err := json.Marshal(
		resolution.ResolvedFrom,
	)
	if err != nil {
		return models.ScheduledCallback{}, fmt.Errorf(
			"callback: marshal resolution: %w",
			err,
		)
	}

	confidence :=
		resolution.Confidence

	return s.repo.Create(
		ctx,
		repository.CreateCallbackInput{
			CallID:               callID,
			LeadID:               leadID,
			RequestedTimeText:    strings.TrimSpace(requestedText),
			ScheduledFor:         resolution.ScheduledFor,
			Timezone:             resolution.Timezone,
			ResolutionConfidence: &confidence,
			ResolutionSource:     resolution.ResolutionSource,
			ResolvedFrom:         resolvedFrom,
			Status:               status,
			CallbackActionID:     actionID,
		},
	)
}

func (s *CallbackService) List(
	ctx context.Context,
) ([]repository.CallbackSummary, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf(
			"callback service is not configured",
		)
	}

	return s.repo.List(ctx)
}

func resolveTimezone(
	value string,
) *time.Location {
	value = strings.TrimSpace(value)

	if value != "" {
		if location, err :=
			time.LoadLocation(value); err == nil {
			return location
		}
	}

	return mustLoadTimezone(
		"Asia/Kolkata",
	)
}

func mustLoadTimezone(
	name string,
) *time.Location {
	location, err :=
		time.LoadLocation(name)

	if err == nil {
		return location
	}

	return time.FixedZone(
		"IST",
		5*60*60+30*60,
	)
}

func hasCallbackIntent(
	text string,
) bool {
	phrases := []string{
		"call me back",
		"call back",
		"call me",
		"call tomorrow",
		"call next week",
		"call next month",
		"call later",
		"contact me",
		"contact me tomorrow",
		"contact me later",
		"call me in",
		"give me a call",
		"get back to me",
		"reach out to me",

		"मुझे कॉल",
		"मुझे कल कॉल",
		"मुझे वापस कॉल",
		"कल कॉल करना",
		"बाद में कॉल करना",
		"फिर कॉल करना",
		"मुझे फिर कॉल",
		"मुझे संपर्क",

		"రేపు కాల్ చేయండి",
		"తర్వాత కాల్ చేయండి",
		"మళ్లీ కాల్ చేయండి",
		"నాకు కాల్",
		"నాకు తిరిగి కాల్",
	}

	for _, phrase := range phrases {
		if strings.Contains(
			text,
			phrase,
		) {
			return true
		}
	}

	return false
}

func resolveDate(
	text string,
	now time.Time,
) (time.Time, float64, string, bool) {
	switch {
	case strings.Contains(
		text,
		"day after tomorrow",
	),
		strings.Contains(text, "परसों"),
		strings.Contains(text, "ఎల్లుండి"):
		return now.AddDate(
				0,
				0,
				2,
			),
			0.97,
			"day_after_tomorrow",
			true

	case strings.Contains(text, "tomorrow"),
		strings.Contains(text, "कल"),
		strings.Contains(text, "రేపు"):
		return now.AddDate(
				0,
				0,
				1,
			),
			0.97,
			"tomorrow",
			true

	case strings.Contains(text, "next week"),
		strings.Contains(text, "अगले हफ्ते"),
		strings.Contains(text, "अगले सप्ताह"),
		strings.Contains(text, "వచ్చే వారం"):
		return now.AddDate(
				0,
				0,
				7,
			),
			0.90,
			"next_week",
			true

	case strings.Contains(text, "next month"),
		strings.Contains(text, "अगले महीने"),
		strings.Contains(text, "వచ్చే నెల"):
		return now.AddDate(
				0,
				1,
				0,
			),
			0.88,
			"next_month",
			true
	}

	if hours, ok :=
		extractRelativeHours(text); ok {
		return now.Add(
				time.Duration(hours) *
					time.Hour,
			),
			0.94,
			"relative_hours",
			true
	}

	if minutes, ok :=
		extractRelativeMinutes(text); ok {
		return now.Add(
				time.Duration(minutes) *
					time.Minute,
			),
			0.94,
			"relative_minutes",
			true
	}

	if weekday, ok :=
		extractWeekday(text); ok {
		days := daysUntilWeekday(
			now.Weekday(),
			weekday,
		)

		if days == 0 {
			days = 7
		}

		return now.AddDate(
				0,
				0,
				days,
			),
			0.91,
			"weekday",
			true
	}

	if strings.Contains(text, "later") ||
		strings.Contains(text, "बाद में") ||
		strings.Contains(text, "बाद") ||
		strings.Contains(text, "తర్వాత") {
		return now.Add(
				2 * time.Hour,
			),
			0.68,
			"later_default",
			true
	}

	return now,
		0,
		"ambiguous",
		false
}

func resolveTime(
	text string,
) (int, int, float64, string, bool) {
	if hour, minute, ok :=
		extractClockTime(text); ok {
		return hour,
			minute,
			0.98,
			"explicit_time",
			true
	}

	switch {
	case strings.Contains(text, "morning"),
		strings.Contains(text, "सुबह"),
		strings.Contains(text, "ఉదయం"):
		return 10,
			0,
			0.90,
			"morning_default",
			true

	case strings.Contains(text, "afternoon"),
		strings.Contains(text, "दोपहर"),
		strings.Contains(text, "మధ్యాహ్నం"):
		return 15,
			0,
			0.89,
			"afternoon_default",
			true

	case strings.Contains(text, "evening"),
		strings.Contains(text, "शाम"),
		strings.Contains(text, "సాయంత్రం"):
		return 19,
			0,
			0.89,
			"evening_default",
			true

	case strings.Contains(text, "night"),
		strings.Contains(text, "रात"),
		strings.Contains(text, "రాత్రి"):
		return 20,
			0,
			0.86,
			"night_default",
			true
	}

	return 0,
		0,
		0,
		"ambiguous",
		false
}

func extractClockTime(
	text string,
) (int, int, bool) {
	pattern := regexp.MustCompile(
		`(?i)\b([0-9]{1,2})(?::([0-9]{2}))?\s*(am|pm)\b`,
	)

	match := pattern.FindStringSubmatch(
		text,
	)

	if len(match) < 4 {
		return 0, 0, false
	}

	hour, err :=
		strconv.Atoi(match[1])

	if err != nil {
		return 0, 0, false
	}

	minute := 0

	if match[2] != "" {
		minute, err =
			strconv.Atoi(match[2])

		if err != nil {
			return 0, 0, false
		}
	}

	period :=
		strings.ToLower(match[3])

	if period == "pm" &&
		hour < 12 {
		hour += 12
	}

	if period == "am" &&
		hour == 12 {
		hour = 0
	}

	if hour > 23 ||
		minute > 59 {
		return 0, 0, false
	}

	return hour, minute, true
}

func extractRelativeHours(
	text string,
) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(
			`(?i)\bin\s+([0-9]{1,2})\s+hours?\b`,
		),
		regexp.MustCompile(
			`(?i)\bafter\s+([0-9]{1,2})\s+hours?\b`,
		),
		regexp.MustCompile(
			`(?i)\b([0-9]{1,2})\s+hours?\s+from\s+now\b`,
		),
	}

	for _, pattern := range patterns {
		match :=
			pattern.FindStringSubmatch(
				text,
			)

		if len(match) < 2 {
			continue
		}

		hours, err :=
			strconv.Atoi(match[1])

		if err != nil ||
			hours <= 0 ||
			hours > 168 {
			continue
		}

		return hours, true
	}

	return 0, false
}

func extractRelativeMinutes(
	text string,
) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(
			`(?i)\bin\s+([0-9]{1,3})\s+minutes?\b`,
		),
		regexp.MustCompile(
			`(?i)\bafter\s+([0-9]{1,3})\s+minutes?\b`,
		),
	}

	for _, pattern := range patterns {
		match :=
			pattern.FindStringSubmatch(
				text,
			)

		if len(match) < 2 {
			continue
		}

		minutes, err :=
			strconv.Atoi(match[1])

		if err != nil ||
			minutes <= 0 ||
			minutes > 10080 {
			continue
		}

		return minutes, true
	}

	return 0, false
}

func extractWeekday(
	text string,
) (time.Weekday, bool) {
	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,

		"रविवार":   time.Sunday,
		"सोमवार":   time.Monday,
		"मंगलवार":  time.Tuesday,
		"बुधवार":   time.Wednesday,
		"गुरुवार":  time.Thursday,
		"शुक्रवार": time.Friday,
		"शनिवार":   time.Saturday,

		"ఆదివారం":   time.Sunday,
		"సోమవారం":   time.Monday,
		"మంగళవారం":  time.Tuesday,
		"బుధవారం":   time.Wednesday,
		"గురువారం":  time.Thursday,
		"శుక్రవారం": time.Friday,
		"శనివారం":   time.Saturday,
	}

	for name, weekday := range weekdays {
		if strings.Contains(
			text,
			name,
		) {
			return weekday, true
		}
	}

	return time.Sunday, false
}

func daysUntilWeekday(
	current time.Weekday,
	target time.Weekday,
) int {
	days := int(target - current)

	if days < 0 {
		days += 7
	}

	return days
}

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

type RelativeTime struct {
	Minutes int
	Hours   int
	Days    int
	Weeks   int
	Months  int
	Years   int
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
	normalized := normalizeText(original)
	location := resolveTimezone(timezone)

	now = now.In(location)

	result := CallbackResolution{
		Timezone: location.String(),
		ResolvedFrom: map[string]any{
			"original_text":    original,
			"normalized_text":  normalized,
			"timezone":         location.String(),
			"resolved_now":     now.Format(time.RFC3339),
			"resolved_now_utc": now.UTC().Format(time.RFC3339),
		},
	}

	if normalized == "" {
		return unresolved(
			result,
			"empty_request",
			0,
			"none",
			"none",
		)
	}

	if !hasCallbackIntent(normalized) {
		return unresolved(
			result,
			"no_callback_intent",
			0,
			"none",
			"none",
		)
	}

	if isImmediateRequest(normalized) {
		scheduled := now.Add(time.Minute)

		return resolved(
			result,
			scheduled,
			0.99,
			"immediate",
			"immediate",
			"immediate",
			false,
		)
	}

	if relative, source, ok := extractRelativeTime(normalized); ok {
		scheduled := applyRelativeTime(
			now,
			relative,
		)

		if !scheduled.After(now) {
			return unresolved(
				result,
				"invalid_relative_time",
				0,
				source,
				source,
			)
		}

		result.ResolvedFrom["relative_minutes"] =
			relative.Minutes

		result.ResolvedFrom["relative_hours"] =
			relative.Hours

		result.ResolvedFrom["relative_days"] =
			relative.Days

		result.ResolvedFrom["relative_weeks"] =
			relative.Weeks

		result.ResolvedFrom["relative_months"] =
			relative.Months

		result.ResolvedFrom["relative_years"] =
			relative.Years

		if hasExplicitClockTime(normalized) {
			hour,
				minute,
				_,
				timeSource,
				timeFound,
				timeNeedsConfirmation :=
				resolveTime(
					normalized,
					now,
				)

			if timeFound {
				scheduled = time.Date(
					scheduled.Year(),
					scheduled.Month(),
					scheduled.Day(),
					hour,
					minute,
					0,
					0,
					location,
				)

				if !scheduled.After(now) {
					return unresolved(
						result,
						"relative_time_in_past",
						0,
						source,
						timeSource,
					)
				}

				confidence := 0.99

				if timeNeedsConfirmation {
					confidence = 0.75
				}

				return resolved(
					result,
					scheduled,
					confidence,
					source+"+"+timeSource,
					source,
					timeSource,
					timeNeedsConfirmation,
				)
			}
		}

		return resolved(
			result,
			scheduled,
			0.99,
			source,
			source,
			source,
			false,
		)
	}

	if isLaterRequest(normalized) &&
		!hasAnyTimeExpression(normalized) {
		return unresolved(
			result,
			"later_without_time",
			0.65,
			"later",
			"missing",
		)
	}

	base,
		dateConfidence,
		dateSource,
		dateFound :=
		resolveDate(
			normalized,
			now,
		)

	if !dateFound {
		base = now
		dateConfidence = 0.90
		dateSource = "today_default"
	}

	hour,
		minute,
		timeConfidence,
		timeSource,
		timeFound,
		timeNeedsConfirmation :=
		resolveTime(
			normalized,
			now,
		)

	if !timeFound {
		return unresolved(
			result,
			dateSource+"+missing_time",
			dateConfidence,
			dateSource,
			"missing",
		)
	}

	scheduled := time.Date(
		base.Year(),
		base.Month(),
		base.Day(),
		hour,
		minute,
		0,
		0,
		location,
	)

	if !scheduled.After(now) {
		switch dateSource {
		case "tomorrow",
			"day_after_tomorrow",
			"weekday",
			"next_week",
			"next_month",
			"next_year",
			"explicit_date":
			return unresolved(
				result,
				"requested_time_in_past",
				0,
				dateSource,
				timeSource,
			)

		default:
			scheduled = scheduled.AddDate(
				0,
				0,
				1,
			)

			result.ResolvedFrom["rolled_forward"] =
				true
		}
	}

	confidence := calculateConfidence(
		dateSource,
		dateConfidence,
		timeSource,
		timeConfidence,
	)

	needsConfirmation :=
		timeNeedsConfirmation ||
			confidence < 0.78

	return resolved(
		result,
		scheduled,
		confidence,
		dateSource+"+"+timeSource,
		dateSource,
		timeSource,
		needsConfirmation,
	)
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

	if resolution.ScheduledFor == nil {
		return models.ScheduledCallback{}, fmt.Errorf(
			"callback: scheduled time could not be resolved from %q",
			requestedText,
		)
	}

	if resolution.NeedsConfirmation {
		return models.ScheduledCallback{}, fmt.Errorf(
			"callback: confirmation required before scheduling",
		)
	}

	location := resolveTimezone(
		resolution.Timezone,
	)

	if strings.TrimSpace(
		resolution.Timezone,
	) == "" {
		resolution.Timezone =
			location.String()
	}

	scheduled :=
		resolution.ScheduledFor.In(location)

	if !scheduled.After(
		time.Now().In(location),
	) {
		return models.ScheduledCallback{}, fmt.Errorf(
			"callback: scheduled time must be in the future",
		)
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

	resolution.ResolvedFrom["scheduled_local_time"] =
		scheduled.Format(time.RFC3339)

	resolution.ResolvedFrom["scheduled_utc_time"] =
		scheduled.UTC().Format(time.RFC3339)

	resolvedFrom, err :=
		json.Marshal(
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
			ScheduledFor:         &scheduled,
			Timezone:             resolution.Timezone,
			ResolutionConfidence: &confidence,
			ResolutionSource:     resolution.ResolutionSource,
			ResolvedFrom:         resolvedFrom,
			Status:               models.CallbackScheduled,
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

func resolved(
	result CallbackResolution,
	scheduled time.Time,
	confidence float64,
	source string,
	dateSource string,
	timeSource string,
	needsConfirmation bool,
) CallbackResolution {
	result.ScheduledFor = &scheduled
	result.Confidence = confidence
	result.ResolutionSource = source
	result.NeedsConfirmation = needsConfirmation

	result.ResolvedFrom["date_source"] =
		dateSource

	result.ResolvedFrom["time_source"] =
		timeSource

	result.ResolvedFrom["resolved_local_time"] =
		scheduled.Format(time.RFC3339)

	result.ResolvedFrom["resolved_utc_time"] =
		scheduled.UTC().Format(time.RFC3339)

	result.ResolvedFrom["needs_confirmation"] =
		needsConfirmation

	return result
}

func unresolved(
	result CallbackResolution,
	source string,
	confidence float64,
	dateSource string,
	timeSource string,
) CallbackResolution {
	result.ScheduledFor = nil
	result.Confidence = confidence
	result.ResolutionSource = source
	result.NeedsConfirmation = true

	result.ResolvedFrom["date_source"] =
		dateSource

	result.ResolvedFrom["time_source"] =
		timeSource

	result.ResolvedFrom["needs_confirmation"] =
		true

	return result
}

func normalizeText(
	text string,
) string {
	text = strings.ToLower(
		strings.TrimSpace(text),
	)

	text = strings.ReplaceAll(
		text,
		"’",
		"'",
	)

	text = strings.ReplaceAll(
		text,
		"-",
		" ",
	)

	text = strings.Join(
		strings.Fields(text),
		" ",
	)

	replacements := []struct {
		from string
		to   string
	}{
		{"a couple of", "2"},
		{"couple of", "2"},
		{"a few", "3"},
		{"half an hour", "30 minutes"},
		{"half a hour", "30 minutes"},
		{"half hour", "30 minutes"},
		{"quarter of an hour", "15 minutes"},
		{"quarter hour", "15 minutes"},
		{"an hour", "1 hour"},
		{"a hour", "1 hour"},
		{"one", "1"},
		{"two", "2"},
		{"three", "3"},
		{"four", "4"},
		{"five", "5"},
		{"six", "6"},
		{"seven", "7"},
		{"eight", "8"},
		{"nine", "9"},
		{"ten", "10"},
		{"eleven", "11"},
		{"twelve", "12"},
		{"thirteen", "13"},
		{"fourteen", "14"},
		{"fifteen", "15"},
		{"sixteen", "16"},
		{"seventeen", "17"},
		{"eighteen", "18"},
		{"nineteen", "19"},
		{"twenty", "20"},
		{"thirty", "30"},
		{"forty", "40"},
		{"fifty", "50"},
		{"sixty", "60"},
	}

	for _, replacement := range replacements {
		pattern :=
			regexp.MustCompile(
				`(?i)\b` +
					regexp.QuoteMeta(
						replacement.from,
					) +
					`\b`,
			)

		text =
			pattern.ReplaceAllString(
				text,
				replacement.to,
			)
	}

	return strings.Join(
		strings.Fields(text),
		" ",
	)
}

func extractRelativeTime(
	text string,
) (RelativeTime, string, bool) {
	patterns := []struct {
		pattern *regexp.Regexp
		source  string
	}{
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(minutes?|mins?|min)\b`,
			),
			"relative_minutes",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(minutes?|mins?|min)\s+(?:later|from now)\b`,
			),
			"relative_minutes",
		},
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(hours?|hrs?|hr)\b`,
			),
			"relative_hours",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(hours?|hrs?|hr)\s+(?:later|from now)\b`,
			),
			"relative_hours",
		},
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(days?)\b`,
			),
			"relative_days",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(days?)\s+(?:later|from now)\b`,
			),
			"relative_days",
		},
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(weeks?|wks?|wk)\b`,
			),
			"relative_weeks",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(weeks?|wks?|wk)\s+(?:later|from now)\b`,
			),
			"relative_weeks",
		},
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(months?|mos?)\b`,
			),
			"relative_months",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(months?|mos?)\s+(?:later|from now)\b`,
			),
			"relative_months",
		},
		{
			regexp.MustCompile(
				`(?i)\b(?:in|after)\s+([0-9]{1,4})\s*(years?|yrs?|yr)\b`,
			),
			"relative_years",
		},
		{
			regexp.MustCompile(
				`(?i)\b([0-9]{1,4})\s*(years?|yrs?|yr)\s+(?:later|from now)\b`,
			),
			"relative_years",
		},
	}

	for _, item := range patterns {
		match :=
			item.pattern.FindStringSubmatch(text)

		if len(match) < 3 {
			continue
		}

		value, err :=
			strconv.Atoi(match[1])

		if err != nil || value <= 0 {
			continue
		}

		switch item.source {
		case "relative_minutes":
			if value <= 10080 {
				return RelativeTime{
						Minutes: value,
					},
					item.source,
					true
			}

		case "relative_hours":
			if value <= 8760 {
				return RelativeTime{
						Hours: value,
					},
					item.source,
					true
			}

		case "relative_days":
			if value <= 3650 {
				return RelativeTime{
						Days: value,
					},
					item.source,
					true
			}

		case "relative_weeks":
			if value <= 520 {
				return RelativeTime{
						Weeks: value,
					},
					item.source,
					true
			}

		case "relative_months":
			if value <= 120 {
				return RelativeTime{
						Months: value,
					},
					item.source,
					true
			}

		case "relative_years":
			if value <= 10 {
				return RelativeTime{
						Years: value,
					},
					item.source,
					true
			}
		}
	}

	return RelativeTime{},
		"",
		false
}

func applyRelativeTime(
	now time.Time,
	relative RelativeTime,
) time.Time {
	result := now.AddDate(
		relative.Years,
		relative.Months,
		relative.Days+
			relative.Weeks*7,
	)

	result = result.Add(
		time.Duration(relative.Hours) *
			time.Hour,
	)

	result = result.Add(
		time.Duration(relative.Minutes) *
			time.Minute,
	)

	return result
}

func resolveDate(
	text string,
	now time.Time,
) (time.Time, float64, string, bool) {
	if date, ok :=
		extractExplicitDate(
			text,
			now,
		); ok {
		return date,
			0.99,
			"explicit_date",
			true
	}

	switch {
	case strings.Contains(
		text,
		"day after tomorrow",
	),
		strings.Contains(text, "परसों"),
		strings.Contains(text, "परसो"),
		strings.Contains(text, "ఎల్లుండి"):

		return now.AddDate(0, 0, 2),
			0.98,
			"day_after_tomorrow",
			true

	case strings.Contains(text, "tomorrow"),
		strings.Contains(text, "कल"),
		strings.Contains(text, "రేపు"):

		return now.AddDate(0, 0, 1),
			0.98,
			"tomorrow",
			true

	case strings.Contains(text, "today"),
		strings.Contains(text, "आज"),
		strings.Contains(text, "ఈరోజు"):

		return now,
			0.99,
			"today",
			true

	case strings.Contains(text, "tonight"),
		strings.Contains(text, "आज रात"),
		strings.Contains(text, "ఈ రాత్రి"):

		return now,
			0.95,
			"tonight",
			true

	case strings.Contains(text, "next week"),
		strings.Contains(text, "अगले हफ्ते"),
		strings.Contains(text, "अगले सप्ताह"),
		strings.Contains(text, "వచ్చే వారం"):

		return now.AddDate(0, 0, 7),
			0.92,
			"next_week",
			true

	case strings.Contains(text, "next month"),
		strings.Contains(text, "अगले महीने"),
		strings.Contains(text, "వచ్చే నెల"):

		return now.AddDate(0, 1, 0),
			0.92,
			"next_month",
			true

	case strings.Contains(text, "next year"),
		strings.Contains(text, "अगले साल"),
		strings.Contains(text, "वచ్చే సంవత్సరం"):

		return now.AddDate(1, 0, 0),
			0.92,
			"next_year",
			true
	}

	if weekday, next, ok :=
		extractWeekday(text); ok {
		days :=
			int(weekday - now.Weekday())

		if days < 0 {
			days += 7
		}

		if next {
			days += 7

			if days == 7 {
				days = 7
			}
		} else if days == 0 {
			days = 7
		}

		return now.AddDate(0, 0, days),
			0.95,
			"weekday",
			true
	}

	return now,
		0,
		"ambiguous",
		false
}

func extractExplicitDate(
	text string,
	now time.Time,
) (time.Time, bool) {
	location := now.Location()

	numericPatterns := []struct {
		pattern *regexp.Regexp
		layout  string
	}{
		{
			regexp.MustCompile(
				`\b[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}\b`,
			),
			"2006-1-2",
		},
		{
			regexp.MustCompile(
				`\b[0-9]{1,2}/[0-9]{1,2}/[0-9]{4}\b`,
			),
			"2/1/2006",
		},
		{
			regexp.MustCompile(
				`\b[0-9]{1,2}-[0-9]{1,2}-[0-9]{4}\b`,
			),
			"2-1-2006",
		},
	}

	for _, item := range numericPatterns {
		value :=
			item.pattern.FindString(text)

		if value == "" {
			continue
		}

		date, err :=
			time.ParseInLocation(
				item.layout,
				value,
				location,
			)

		if err != nil {
			continue
		}

		return date,
			true
	}

	months := map[string]time.Month{
		"january":   time.January,
		"february":  time.February,
		"march":     time.March,
		"april":     time.April,
		"may":       time.May,
		"june":      time.June,
		"july":      time.July,
		"august":    time.August,
		"september": time.September,
		"october":   time.October,
		"november":  time.November,
		"december":  time.December,
	}

	pattern :=
		regexp.MustCompile(
			`(?i)\b(` +
				strings.Join(
					[]string{
						"january",
						"february",
						"march",
						"april",
						"may",
						"june",
						"july",
						"august",
						"september",
						"october",
						"november",
						"december",
					},
					"|",
				) +
				`)\s+([0-9]{1,2})(?:st|nd|rd|th)?(?:,?\s+([0-9]{4}))?\b`,
		)

	match :=
		pattern.FindStringSubmatch(text)

	if len(match) == 0 {
		return time.Time{},
			false
	}

	month :=
		months[strings.ToLower(match[1])]

	day, err :=
		strconv.Atoi(match[2])

	if err != nil {
		return time.Time{},
			false
	}

	year := now.Year()

	if match[3] != "" {
		year, err =
			strconv.Atoi(match[3])

		if err != nil {
			return time.Time{},
				false
		}
	}

	date := time.Date(
		year,
		month,
		day,
		0,
		0,
		0,
		0,
		location,
	)

	if date.Month() != month ||
		date.Day() != day {
		return time.Time{},
			false
	}

	if match[3] == "" &&
		date.Before(
			time.Date(
				now.Year(),
				now.Month(),
				now.Day(),
				0,
				0,
				0,
				0,
				location,
			),
		) {
		date = date.AddDate(
			1,
			0,
			0,
		)
	}

	return date,
		true
}

func resolveTime(
	text string,
	now time.Time,
) (
	int,
	int,
	float64,
	string,
	bool,
	bool,
) {
	if hour, minute, ok :=
		extractMeridiemTime(text); ok {
		return hour,
			minute,
			0.99,
			"explicit_time",
			true,
			false
	}

	if hour, minute, ok :=
		extract24HourTime(text); ok {
		return hour,
			minute,
			0.99,
			"explicit_time_24h",
			true,
			false
	}

	if hour, minute, ok :=
		extractBareTime(text); ok {
		return hour,
			minute,
			0.75,
			"bare_time",
			true,
			true
	}

	switch {
	case strings.Contains(text, "noon"):
		return 12,
			0,
			0.99,
			"noon",
			true,
			false

	case strings.Contains(text, "midnight"):
		return 0,
			0,
			0.99,
			"midnight",
			true,
			false

	case strings.Contains(text, "morning"),
		strings.Contains(text, "सुबह"),
		strings.Contains(text, "ఉదయం"):

		return 10,
			0,
			0.85,
			"morning_default",
			true,
			true

	case strings.Contains(text, "afternoon"),
		strings.Contains(text, "दोपहर"),
		strings.Contains(text, "మధ్యాహ్నం"):

		return 15,
			0,
			0.85,
			"afternoon_default",
			true,
			true

	case strings.Contains(text, "evening"),
		strings.Contains(text, "शाम"),
		strings.Contains(text, "సాయంత్రం"):

		return 19,
			0,
			0.85,
			"evening_default",
			true,
			true

	case strings.Contains(text, "night"),
		strings.Contains(text, "रात"),
		strings.Contains(text, "రాత్రి"):

		return 20,
			0,
			0.82,
			"night_default",
			true,
			true
	}

	return 0,
		0,
		0,
		"ambiguous",
		false,
		true
}

func extractMeridiemTime(
	text string,
) (int, int, bool) {
	pattern :=
		regexp.MustCompile(
			`(?i)\b([0-9]{1,2})(?::([0-9]{1,2}))?\s*(a\.?m\.?|p\.?m\.?)\b`,
		)

	match :=
		pattern.FindStringSubmatch(text)

	if len(match) < 4 {
		return 0,
			0,
			false
	}

	hour, err :=
		strconv.Atoi(match[1])

	if err != nil ||
		hour < 1 ||
		hour > 12 {
		return 0,
			0,
			false
	}

	minute := 0

	if match[2] != "" {
		minute, err =
			strconv.Atoi(match[2])

		if err != nil ||
			minute > 59 {
			return 0,
				0,
				false
		}
	}

	period :=
		strings.ReplaceAll(
			strings.ToLower(match[3]),
			".",
			"",
		)

	if period == "pm" &&
		hour < 12 {
		hour += 12
	}

	if period == "am" &&
		hour == 12 {
		hour = 0
	}

	return hour,
		minute,
		true
}

func extract24HourTime(
	text string,
) (int, int, bool) {
	pattern :=
		regexp.MustCompile(
			`\b([01]?[0-9]|2[0-3]):([0-5][0-9])\b`,
		)

	match :=
		pattern.FindStringSubmatch(text)

	if len(match) < 3 {
		return 0,
			0,
			false
	}

	hour, err :=
		strconv.Atoi(match[1])

	if err != nil {
		return 0,
			0,
			false
	}

	minute, err :=
		strconv.Atoi(match[2])

	if err != nil {
		return 0,
			0,
			false
	}

	return hour,
		minute,
		true
}

func extractBareTime(
	text string,
) (int, int, bool) {
	pattern :=
		regexp.MustCompile(
			`(?i)\b(?:at|around|by|@)\s*([0-9]{1,2})\b`,
		)

	match :=
		pattern.FindStringSubmatch(text)

	if len(match) < 2 {
		return 0,
			0,
			false
	}

	hour, err :=
		strconv.Atoi(match[1])

	if err != nil ||
		hour < 1 ||
		hour > 12 {
		return 0,
			0,
			false
	}

	return hour,
		0,
		true
}

func hasExplicitClockTime(
	text string,
) bool {
	if _, _, ok :=
		extractMeridiemTime(text); ok {
		return true
	}

	if _, _, ok :=
		extract24HourTime(text); ok {
		return true
	}

	if _, _, ok :=
		extractBareTime(text); ok {
		return true
	}

	return false
}

func hasAnyTimeExpression(
	text string,
) bool {
	if hasExplicitClockTime(text) {
		return true
	}

	keywords := []string{
		"morning",
		"afternoon",
		"evening",
		"night",
		"noon",
		"midnight",
		"सुबह",
		"दोपहर",
		"शाम",
		"रात",
		"ఉదయం",
		"మధ్యాహ్నం",
		"సాయంత్రం",
		"రాత్రి",
	}

	for _, keyword := range keywords {
		if strings.Contains(
			text,
			keyword,
		) {
			return true
		}
	}

	return false
}

func calculateConfidence(
	dateSource string,
	dateConfidence float64,
	timeSource string,
	timeConfidence float64,
) float64 {
	if dateSource == "today_default" &&
		(timeSource == "explicit_time" ||
			timeSource == "explicit_time_24h") {
		return 0.98
	}

	if timeSource == "explicit_time" ||
		timeSource == "explicit_time_24h" {
		return 0.98
	}

	if timeSource == "noon" ||
		timeSource == "midnight" {
		return 0.98
	}

	if timeSource == "bare_time" {
		return 0.75
	}

	if strings.Contains(
		timeSource,
		"_default",
	) {
		return 0.70
	}

	return dateConfidence *
		timeConfidence
}

func resolveTimezone(
	value string,
) *time.Location {
	value = strings.TrimSpace(value)

	if value != "" {
		location, err :=
			time.LoadLocation(value)

		if err == nil {
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
		"callback",
		"call me",
		"call tomorrow",
		"call later",
		"call now",
		"contact me",
		"give me a call",
		"give me a callback",
		"get back to me",
		"reach out to me",
		"ring me",
		"phone me",
		"try me again",
		"call in",
		"call after",
		"call at",
		"call around",
		"call by",

		"मुझे कॉल",
		"मुझे वापस कॉल",
		"कल कॉल",
		"कॉल करना",
		"फिर कॉल",
		"बाद में कॉल",
		"अभी कॉल",

		"నాకు కాల్",
		"తిరిగి కాల్",
		"కాల్ చేయండి",
		"మళ్లీ కాల్",
		"తర్వాత కాల్",
		"ఇప్పుడు కాల్",
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

func isImmediateRequest(
	text string,
) bool {
	phrases := []string{
		"call me now",
		"call now",
		"right now",
		"immediately",
		"asap",
		"straight away",
		"अभी",
		"अभी कॉल",
		"वर्तमान में",
		"वेंटे",
		"వెంటనే",
		"ఇప్పుడు",
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

func isLaterRequest(
	text string,
) bool {
	phrases := []string{
		"later",
		"later on",
		"sometime later",
		"बाद में",
		"बाद",
		"తర్వాత",
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

func extractWeekday(
	text string,
) (
	time.Weekday,
	bool,
	bool,
) {
	weekdays := []struct {
		names   []string
		weekday time.Weekday
	}{
		{
			[]string{
				"sunday",
				"रविवार",
				"ఆదివారం",
			},
			time.Sunday,
		},
		{
			[]string{
				"monday",
				"सोमवार",
				"సోమవారం",
			},
			time.Monday,
		},
		{
			[]string{
				"tuesday",
				"मंगलवार",
				"మంగళవారం",
			},
			time.Tuesday,
		},
		{
			[]string{
				"wednesday",
				"बुधवार",
				"బుధవారం",
			},
			time.Wednesday,
		},
		{
			[]string{
				"thursday",
				"गुरुवार",
				"గురువారం",
			},
			time.Thursday,
		},
		{
			[]string{
				"friday",
				"शुक्रवार",
				"శుక్రవారం",
			},
			time.Friday,
		},
		{
			[]string{
				"saturday",
				"शनिवार",
				"శనివారం",
			},
			time.Saturday,
		},
	}

	for _, item := range weekdays {
		for _, name := range item.names {
			if !strings.Contains(
				text,
				name,
			) {
				continue
			}

			next :=
				strings.Contains(
					text,
					"next "+name,
				) ||
					strings.Contains(
						text,
						"अगले "+name,
					) ||
					strings.Contains(
						text,
						"वाले "+name,
					) ||
					strings.Contains(
						text,
						"వచ్చే "+name,
					)

			return item.weekday,
				next,
				true
		}
	}

	return time.Sunday,
		false,
		false
}

package provider

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

type UsageUnitKind string

const (
	UsageUnitImageCount   UsageUnitKind = "image_count"
	UsageUnitVideoSeconds UsageUnitKind = "video_seconds"
	UsageUnitAudioSeconds UsageUnitKind = "audio_seconds"
)

type JobUsage struct {
	UnitKind        UsageUnitKind `json:"unit_kind"`
	RequestedUnits  int64         `json:"requested_units"`
	BilledUnits     int64         `json:"billed_units"`
	Currency        string        `json:"currency"`
	ActualCostMinor *int64        `json:"actual_cost_minor"`
	MeasuredAt      time.Time     `json:"measured_at"`
}

func (u JobUsage) Validate() error {
	if u.UnitKind != UsageUnitImageCount && u.UnitKind != UsageUnitVideoSeconds && u.UnitKind != UsageUnitAudioSeconds {
		return fmt.Errorf("provider usage unit kind is invalid")
	}
	if u.RequestedUnits < 0 || u.BilledUnits < 0 || len(u.Currency) != 3 || strings.ToUpper(u.Currency) != u.Currency || u.MeasuredAt.IsZero() {
		return fmt.Errorf("provider usage is incomplete")
	}
	if u.ActualCostMinor != nil && *u.ActualCostMinor < 0 {
		return fmt.Errorf("provider actual cost must not be negative")
	}
	return nil
}

type JobEvent struct {
	Ordinal     int       `json:"ordinal"`
	Stage       string    `json:"stage"`
	SafeMessage string    `json:"safe_message"`
	ErrorCode   string    `json:"error_code,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func sanitizeJobEvent(event JobEvent) JobEvent {
	event.Stage = boundedSafeToken(event.Stage, 64, "unknown")
	event.ErrorCode = boundedSafeToken(event.ErrorCode, 128, "")
	event.SafeMessage = sanitizeJobEventMessage(event.SafeMessage)
	return event
}

func sanitizeJobEventMessage(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"authorization", "bearer ", "api_key", "apikey", "secret", "prompt=", "prompt:", "http://", "https://", "s3://", "oss://", "bucket", "object_key", "storage_key"} {
		if strings.Contains(lower, marker) {
			return "Provider event details were redacted."
		}
	}
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\t' {
			return -1
		}
		return r
	}, trimmed))
	if len(runes) > 512 {
		runes = runes[:512]
	}
	if len(runes) == 0 {
		return "Provider state changed."
	}
	return string(runes)
}

func boundedSafeToken(value string, limit int, fallback string) string {
	value = strings.TrimSpace(value)
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.') {
			return fallback
		}
	}
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	if len(runes) == 0 {
		return fallback
	}
	return string(runes)
}

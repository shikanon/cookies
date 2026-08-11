package creative

import (
	"fmt"
	"strings"
	"time"
)

// RenderUsage is an owner-side billing fact. A nil ActualCostMinor is not zero:
// it must carry the reason that an actual cost is unavailable.
type RenderUsage struct {
	Currency          string
	ActualCostMinor   *int64
	UnavailableReason *string
	MeasuredAt        time.Time
}

func (u RenderUsage) Validate() error {
	if len(u.Currency) != 3 || strings.ToUpper(u.Currency) != u.Currency || u.MeasuredAt.IsZero() {
		return fmt.Errorf("creative render usage is incomplete")
	}
	if u.ActualCostMinor != nil {
		if *u.ActualCostMinor < 0 || u.UnavailableReason != nil {
			return fmt.Errorf("creative render actual cost is invalid")
		}
		return nil
	}
	if u.UnavailableReason == nil || strings.TrimSpace(*u.UnavailableReason) == "" || len([]rune(*u.UnavailableReason)) > 500 {
		return fmt.Errorf("creative render unavailable cost reason is required")
	}
	return nil
}

// RenderEvent is append-only owner state. SafeMessage is fixed by the owner;
// raw renderer messages, prompts, URLs and storage locations are never stored.
type RenderEvent struct {
	Ordinal     int
	Stage       string
	SafeMessage string
	ErrorCode   string
	OccurredAt  time.Time
}

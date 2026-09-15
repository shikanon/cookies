package connector

import "testing"

func TestBiddingHistoryKeepsBidLevelsAndUnitsSeparate(t *testing.T) {
	h := ReadBiddingHistory(map[string]any{"project_bid": "1.23", "ad_bid": 0.5, "currency": "CNY", "external_action": "19"})
	if h.ProjectBidMinor == nil || *h.ProjectBidMinor != 123 || h.UnitBidMinor == nil || *h.UnitBidMinor != 50 {
		t.Fatalf("wrong money units: %+v", h)
	}
	h = ReadBiddingHistory(map[string]any{"bid": 2, "bid_minor": 200})
	if h.ProjectBidMinor != nil || h.UnitBidMinor != nil {
		t.Fatal("ambiguous bid used as an explicit project or unit bid")
	}
}

package connector

// BiddingHistory separates project and promotion bids. An unqualified bid is
// deliberately excluded because it does not establish which level takes effect.
type BiddingHistory struct {
	ProjectBidMinor    *int64
	UnitBidMinor       *int64
	Currency           string
	ChargingMode       string
	OptimizationTarget string
	DeliveryMode       string
	BiddingStrategy    string
	MarketingPurpose   string
	Carrier            string
}

func ReadBiddingHistory(values map[string]any) BiddingHistory {
	text := func(keys ...string) string {
		if value := controlledString(values, keys...); value != nil {
			return *value
		}
		return ""
	}
	return BiddingHistory{
		ProjectBidMinor: currencyMinor(values, []string{"project_bid_minor"}, []string{"project_bid"}),
		UnitBidMinor:    currencyMinor(values, []string{"unit_bid_minor"}, []string{"ad_bid"}),
		Currency:        text("currency"), ChargingMode: text("charging_mode", "ad_pricing_name"),
		OptimizationTarget: text("external_action", "optimization_target"),
		DeliveryMode:       text("delivery_mode"), BiddingStrategy: text("bidding_strategy"),
		MarketingPurpose: text("marketing_purpose"), Carrier: text("carrier"),
	}
}

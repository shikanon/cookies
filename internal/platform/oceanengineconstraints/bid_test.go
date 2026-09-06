package oceanengineconstraints

import "testing"

func TestResolveChargingModeFromOptimizationTarget(t *testing.T) {
	for _, test := range []struct {
		target, fallback, want string
	}{
		{"impression", "CPC", "CPM"},
		{"click", "CPM", "CPC"},
		{"button_jump", "CPC", "OCPM"},
		{"button_jump", "OCPC", "OCPC"},
		{"", "ocpc", "OCPC"},
	} {
		if got := ResolveChargingMode(test.target, test.fallback); got != test.want {
			t.Fatalf("ResolveChargingMode(%q, %q) = %q, want %q", test.target, test.fallback, got, test.want)
		}
	}
}

func TestResolveBidConstraint(t *testing.T) {
	cpm, err := Resolve("CPM", 30000)
	if err != nil || cpm.MinimumMinor != 400 || cpm.MaximumMinor != 10000 {
		t.Fatalf("CPM constraint = %#v, %v", cpm, err)
	}
	ocpm, err := Resolve("OCPM", 30000)
	if err != nil || ocpm.MinimumMinor != 1 || ocpm.MaximumMinor != 30000 || ocpm.MaximumSource != "daily_budget" {
		t.Fatalf("OCPM constraint = %#v, %v", ocpm, err)
	}
	if err := ValidateBid(1, cpm); err == nil {
		t.Fatal("CPM bid below CNY 4 must fail")
	}
}

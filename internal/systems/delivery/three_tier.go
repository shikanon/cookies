package delivery

import (
	"fmt"
	"strings"
	"time"
)

const ThreeTierSchema = "delivery-three-tier/v1"

// ThreeTierConfiguration is a complete, mock-only delivery snapshot. It is
// deliberately attached to a PlanVersion rather than a mutable plan row.
type ThreeTierConfiguration struct {
	Schema          string           `json:"schema"`
	Source          Source           `json:"source"`
	Scenario        ThreeTierFixture `json:"scenario"`
	FixtureScenario ThreeTierFixture `json:"fixture_scenario"`
	GeneratedAt     time.Time        `json:"generated_at"`
	Evidence        []string         `json:"evidence"`
	Groups          []ThreeTierGroup `json:"groups"`
}
type ThreeTierGroup struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Fields []ThreeTierField `json:"fields"`
	Plans  []ThreeTierPlan  `json:"plans"`
}
type ThreeTierPlan struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Fields    []ThreeTierField    `json:"fields"`
	Creatives []ThreeTierCreative `json:"creatives"`
}
type ThreeTierCreative struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Fields []ThreeTierField `json:"fields"`
}
type ThreeTierValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}
type ThreeTierField struct {
	Key              string          `json:"key"`
	Label            string          `json:"label"`
	Recommended      ThreeTierValue  `json:"recommended"`
	Manual           *ThreeTierValue `json:"manual,omitempty"`
	Effective        ThreeTierValue  `json:"effective"`
	Source           string          `json:"source"`
	SourceRefs       []string        `json:"source_refs"`
	EffectiveSource  string          `json:"effective_source"`
	Dependency       string          `json:"dependency,omitempty"`
	DependencyRefs   []string        `json:"dependency_refs"`
	Risk             string          `json:"risk,omitempty"`
	RiskRefs         []string        `json:"risk_refs"`
	EvidenceRefs     []string        `json:"evidence_refs"`
	MockRequired     bool            `json:"mock_required"`
	PlatformRequired bool            `json:"platform_required"`
	PlatformStatus   string          `json:"platform_status"`
	Editable         bool            `json:"editable"`
	Confirmed        bool            `json:"confirmation"`
}

func (c ThreeTierConfiguration) Validate() error {
	if c.Schema != "delivery-three-tier/v1" || c.Source != SourceMock || strings.TrimSpace(string(c.Scenario)) == "" || c.FixtureScenario != c.Scenario || c.GeneratedAt.IsZero() || len(c.Evidence) == 0 {
		return fmt.Errorf("three_tier_configuration must be a mock delivery-three-tier/v1 snapshot")
	}
	if len(c.Groups) == 0 {
		return fmt.Errorf("three_tier_configuration.groups is required")
	}
	groups := map[string]bool{}
	for _, g := range c.Groups {
		if g.ID == "" || g.Name == "" || groups[g.ID] || len(g.Plans) == 0 {
			return fmt.Errorf("three_tier_configuration groups must have unique ids and names and plans")
		}
		groups[g.ID] = true
		if err := validateLayerFields(g.Fields); err != nil {
			return err
		}
		plans := map[string]bool{}
		for _, p := range g.Plans {
			if p.ID == "" || p.Name == "" || plans[p.ID] || len(p.Creatives) == 0 {
				return fmt.Errorf("three_tier_configuration plans must have unique ids and creatives")
			}
			plans[p.ID] = true
			if err := validateLayerFields(p.Fields); err != nil {
				return err
			}
			creatives := map[string]bool{}
			for _, cr := range p.Creatives {
				if cr.ID == "" || cr.Name == "" || creatives[cr.ID] || len(cr.Fields) == 0 {
					return fmt.Errorf("three_tier_configuration creatives must have unique ids and fields")
				}
				creatives[cr.ID] = true
				if err := validateLayerFields(cr.Fields); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func validateLayerFields(values []ThreeTierField) error {
	if len(values) == 0 {
		return fmt.Errorf("three_tier_configuration layer fields are required")
	}
	seen := map[string]bool{}
	for _, f := range values {
		if f.Key == "" || f.Label == "" || seen[f.Key] || f.Recommended.Type == "" || f.Effective.Type == "" || f.Source == "" || len(f.SourceRefs) == 0 || f.EffectiveSource == "" || f.PlatformStatus == "" || len(f.RiskRefs) == 0 || len(f.EvidenceRefs) == 0 {
			return fmt.Errorf("three_tier_configuration field %q is invalid", f.Key)
		}
		seen[f.Key] = true
		if f.PlatformRequired && f.PlatformStatus != "pending" {
			return fmt.Errorf("real-platform field %q must remain pending", f.Key)
		}
		if f.Manual != nil && !f.Editable {
			return fmt.Errorf("manual value requires editable field %q", f.Key)
		}
	}
	return nil
}
func cloneThreeTierConfiguration(v *ThreeTierConfiguration) *ThreeTierConfiguration {
	if v == nil {
		return nil
	}
	b := *v
	b.Evidence = append([]string(nil), v.Evidence...)
	b.Groups = append([]ThreeTierGroup(nil), v.Groups...)
	for i := range b.Groups {
		b.Groups[i].Fields = cloneThreeTierFields(v.Groups[i].Fields)
		b.Groups[i].Plans = append([]ThreeTierPlan(nil), v.Groups[i].Plans...)
		for j := range b.Groups[i].Plans {
			b.Groups[i].Plans[j].Fields = cloneThreeTierFields(v.Groups[i].Plans[j].Fields)
			b.Groups[i].Plans[j].Creatives = append([]ThreeTierCreative(nil), v.Groups[i].Plans[j].Creatives...)
			for k := range b.Groups[i].Plans[j].Creatives {
				b.Groups[i].Plans[j].Creatives[k].Fields = cloneThreeTierFields(v.Groups[i].Plans[j].Creatives[k].Fields)
			}
		}
	}
	return &b
}
func cloneThreeTierFields(values []ThreeTierField) []ThreeTierField {
	out := append([]ThreeTierField(nil), values...)
	for i := range out {
		f := &out[i]
		f.EvidenceRefs = append([]string(nil), f.EvidenceRefs...)
		f.SourceRefs = append([]string(nil), f.SourceRefs...)
		f.DependencyRefs = append([]string(nil), f.DependencyRefs...)
		f.RiskRefs = append([]string(nil), f.RiskRefs...)
		if f.Manual != nil {
			m := *f.Manual
			f.Manual = &m
		}
	}
	return out
}

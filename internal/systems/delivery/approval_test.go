package delivery

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestCanonicalJSONHashIgnoresObjectMemberOrderForDeliveryPayloads(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"name": "黄金计划",
		"advertiser": map[string]any{
			"id": "advertiser_1", "name": "Mock", "platform": "ocean_engine",
		},
		"budget": map[string]any{"total_minor": 300000, "currency": "CNY"},
	}
	right := map[string]any{
		"budget": map[string]any{"currency": "CNY", "total_minor": 300000},
		"advertiser": map[string]any{
			"platform": "ocean_engine", "name": "Mock", "id": "advertiser_1",
		},
		"name": "黄金计划",
	}
	leftHash, err := contract.CanonicalJSONHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := contract.CanonicalJSONHash(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("member order changed the hash: %s != %s", leftHash, rightHash)
	}
}

func TestPlanCanonicalHashCoversEveryDeliveryBusinessField(t *testing.T) {
	t.Parallel()
	base := canonicalTestVersion(t)
	baseHash, err := PlanCanonicalHash(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*DeliveryPlanVersion)
	}{
		{name: "name", mutate: func(value *DeliveryPlanVersion) { value.Name += " 2" }},
		{name: "objective", mutate: func(value *DeliveryPlanVersion) { value.Objective += " 2" }},
		{name: "advertiser id", mutate: func(value *DeliveryPlanVersion) { value.Advertiser.ID += "_2" }},
		{name: "advertiser name", mutate: func(value *DeliveryPlanVersion) { value.Advertiser.Name += " 2" }},
		{name: "advertiser platform", mutate: func(value *DeliveryPlanVersion) { value.Advertiser.Platform = "other_mock" }},
		{name: "budget amount", mutate: func(value *DeliveryPlanVersion) { value.Budget.TotalMinor++ }},
		{name: "budget currency", mutate: func(value *DeliveryPlanVersion) { value.Budget.Currency = "USD" }},
		{name: "schedule start", mutate: func(value *DeliveryPlanVersion) { value.Schedule.StartAt = value.Schedule.StartAt.Add(time.Minute) }},
		{name: "schedule end", mutate: func(value *DeliveryPlanVersion) { value.Schedule.EndAt = value.Schedule.EndAt.Add(time.Minute) }},
		{name: "schedule timezone", mutate: func(value *DeliveryPlanVersion) { value.Schedule.Timezone = "UTC" }},
		{name: "tracking landing page", mutate: func(value *DeliveryPlanVersion) { value.Tracking.LandingPage += "/v2" }},
		{name: "tracking pixel", mutate: func(value *DeliveryPlanVersion) { value.Tracking.PixelID += "_2" }},
		{name: "tracking event", mutate: func(value *DeliveryPlanVersion) { value.Tracking.ConversionEvent += "_2" }},
		{name: "creative asset", mutate: func(value *DeliveryPlanVersion) { value.CreativeReferences[0].AssetID += "_2" }},
		{name: "creative version", mutate: func(value *DeliveryPlanVersion) { value.CreativeReferences[0].Version++ }},
		{name: "creative confirmation", mutate: func(value *DeliveryPlanVersion) { value.CreativeReferences[0].Confirmed = false }},
		{name: "source strategy version", mutate: func(value *DeliveryPlanVersion) { value.SourceStrategyVersion += "_2" }},
		{name: "source boundary", mutate: func(value *DeliveryPlanVersion) { value.Source = "other" }},
		{name: "platform boundary", mutate: func(value *DeliveryPlanVersion) { value.Platform = "other_mock" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := cloneVersion(base)
			testCase.mutate(&changed)
			hash, err := PlanCanonicalHash(changed)
			if err != nil {
				t.Fatal(err)
			}
			if hash == baseHash {
				t.Fatalf("%s did not change the canonical hash", testCase.name)
			}
		})
	}
}

func TestPlanCanonicalHashExcludesAuditAndDisplayMetadata(t *testing.T) {
	t.Parallel()
	base := canonicalTestVersion(t)
	baseHash, err := PlanCanonicalHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.CreatedAt = base.CreatedAt.Add(48 * time.Hour)
	base.CreatedBy = contract.Principal{Kind: contract.PrincipalService, ID: "service_2"}
	base.CanonicalHash = "not-part-of-payload"
	hash, err := PlanCanonicalHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if hash != baseHash {
		t.Fatalf("audit metadata changed the content hash: %s != %s", hash, baseHash)
	}
}

func TestApprovalActionHashBindsEveryApprovedActionField(t *testing.T) {
	t.Parallel()
	base := DeliveryApproval{
		OrganizationID: "org_a", ProjectID: "project_a",
		PlanID: "plan_a", PlanVersion: 2,
		ChangeSetID: "change_a", ChangeSetVersion: 3,
		PlanCanonicalHash:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TargetSnapshotHash:         "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ConfigurationSchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "configuration_a", ConfigurationVersion: 2,
		ConfigurationPlatform: DeliveryPlatformOceanEngine, ConfigurationProfileVersion: OceanEngineConfigurationProfileV1,
		ConfigurationCanonicalHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		IntentSchemaVersion:        DeliveryIntentSchemaV1, IntentID: "intent_a", IntentVersion: 2,
		IntentCanonicalHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Action:              ApprovalActionExecute, Scope: ApprovalScopeExecuteMock,
		BudgetLimitMinor: 300000, Currency: "CNY",
	}
	baseHash, err := ApprovalActionHash(base)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*DeliveryApproval)
	}{
		{name: "organization", mutate: func(value *DeliveryApproval) { value.OrganizationID = "org_b" }},
		{name: "project", mutate: func(value *DeliveryApproval) { value.ProjectID = "project_b" }},
		{name: "plan id", mutate: func(value *DeliveryApproval) { value.PlanID = "plan_b" }},
		{name: "plan version", mutate: func(value *DeliveryApproval) { value.PlanVersion++ }},
		{name: "change set id", mutate: func(value *DeliveryApproval) { value.ChangeSetID = "change_b" }},
		{name: "change set version", mutate: func(value *DeliveryApproval) { value.ChangeSetVersion++ }},
		{name: "plan hash", mutate: func(value *DeliveryApproval) {
			value.PlanCanonicalHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{name: "target hash", mutate: func(value *DeliveryApproval) {
			value.TargetSnapshotHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		}},
		{name: "configuration id", mutate: func(value *DeliveryApproval) { value.ConfigurationID = "configuration_b" }},
		{name: "configuration version", mutate: func(value *DeliveryApproval) { value.ConfigurationVersion++ }},
		{name: "configuration platform", mutate: func(value *DeliveryApproval) { value.ConfigurationPlatform = DeliveryPlatformMagneticEngine }},
		{name: "configuration profile", mutate: func(value *DeliveryApproval) {
			value.ConfigurationProfileVersion = MagneticEngineConfigurationProfileV1
		}},
		{name: "configuration hash", mutate: func(value *DeliveryApproval) {
			value.ConfigurationCanonicalHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		}},
		{name: "intent id", mutate: func(value *DeliveryApproval) { value.IntentID = "intent_b" }},
		{name: "intent version", mutate: func(value *DeliveryApproval) { value.IntentVersion++ }},
		{name: "intent hash", mutate: func(value *DeliveryApproval) {
			value.IntentCanonicalHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		}},
		{name: "action", mutate: func(value *DeliveryApproval) { value.Action = "pause" }},
		{name: "scope", mutate: func(value *DeliveryApproval) { value.Scope = "execute_real" }},
		{name: "budget", mutate: func(value *DeliveryApproval) { value.BudgetLimitMinor++ }},
		{name: "currency", mutate: func(value *DeliveryApproval) { value.Currency = "USD" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := base
			testCase.mutate(&changed)
			hash, err := ApprovalActionHash(changed)
			if err != nil {
				t.Fatal(err)
			}
			if hash == baseHash {
				t.Fatalf("%s did not change the action hash", testCase.name)
			}
		})
	}
}

func TestBackfillPlanVersionPayloadIsDeterministicAndUsesCanonicalizer(t *testing.T) {
	t.Parallel()
	version := canonicalTestVersion(t)
	version.Platform, version.CanonicalHash = "", ""
	payload, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	row := planVersionBackfillRow{
		OrganizationID: version.OrganizationID,
		ProjectID:      version.ProjectID,
		PlanID:         version.PlanID,
		VersionNumber:  version.VersionNumber,
		Platform:       "ocean_engine_mock",
		ConfigJSON:     payload,
	}
	firstPayload, firstHash, err := backfillPlanVersionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	row.ConfigJSON = firstPayload
	row.CanonicalHash = sql.NullString{String: firstHash, Valid: true}
	secondPayload, secondHash, err := backfillPlanVersionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || string(firstPayload) != string(secondPayload) {
		t.Fatalf("backfill is not deterministic:\n%s\n%s", firstPayload, secondPayload)
	}
	var backfilled DeliveryPlanVersion
	if err := json.Unmarshal(firstPayload, &backfilled); err != nil {
		t.Fatal(err)
	}
	if backfilled.CanonicalHash != firstHash || backfilled.Platform != "ocean_engine_mock" {
		t.Fatalf("unexpected backfilled version: %#v", backfilled)
	}
}

func TestBackfillRejectsAnExistingMismatchedCanonicalHash(t *testing.T) {
	t.Parallel()
	row := planVersionBackfillRow{
		PlanID:        "plan_a",
		VersionNumber: 2,
		CanonicalHash: sql.NullString{
			String: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Valid:  true,
		},
	}
	err := validateExistingCanonicalHash(
		row,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	)
	if err == nil {
		t.Fatal("existing canonical hash mismatch should stop the backfill")
	}
}

func TestLegacyApprovalBackfillCreatesDeterministicImmutableAuthority(t *testing.T) {
	t.Parallel()
	version := canonicalTestVersion(t)
	payload, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	approvedAt := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	row := legacyApprovalBackfillRow{
		OrganizationID:    version.OrganizationID,
		ProjectID:         version.ProjectID,
		PlanID:            version.PlanID,
		PlanVersion:       int64(version.VersionNumber),
		ChangeSetID:       "change_a",
		ChangeSetVersion:  4,
		Status:            ChangeSetExecuted,
		ApprovedBy:        "user_a",
		ApprovedAt:        approvedAt,
		PlanCanonicalHash: version.CanonicalHash,
		ConfigJSON:        payload,
	}
	first, err := approvalFromLegacyProjection(row)
	if err != nil {
		t.Fatal(err)
	}
	second, err := approvalFromLegacyProjection(row)
	if err != nil {
		t.Fatal(err)
	}
	if !sameApproval(first, second) {
		t.Fatalf("legacy approval backfill is not deterministic:\n%#v\n%#v", first, second)
	}
	if first.ChangeSetVersion != 3 ||
		first.ExpiresAt.Sub(first.ApprovedAt) != ApprovalTTL ||
		first.Scope != ApprovalScopeExecuteMock ||
		first.Source != SourceMock ||
		first.ActionHash == "" {
		t.Fatalf("unexpected legacy approval: %#v", first)
	}
}

func canonicalTestVersion(t *testing.T) DeliveryPlanVersion {
	t.Helper()
	plan := DeliveryPlan{
		ID: "plan_a", OrganizationID: "org_a", ProjectID: "project_a",
		Platform: "ocean_engine_mock",
	}
	version, err := versionFromDraft(
		plan,
		1,
		goldenDraft(),
		contract.Principal{Kind: contract.PrincipalUser, ID: "user_a"},
		time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestMySQLPlatformRuntimeRoundTripAndLegacyUpgradeCompatibility(t *testing.T) {
	dsn := os.Getenv("COOKIES_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("COOKIES_TEST_MYSQL_DSN is not configured")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	organizationID := contract.OrganizationID("org_" + suffix)
	projectID := contract.ProjectID("project_" + suffix)
	if _, err := db.ExecContext(ctx, `INSERT INTO organizations (id,name,status) VALUES (?,?,'active')`, organizationID, "Delivery runtime integration"); err != nil {
		t.Fatalf("create organization fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO projects (id,organization_id,name,status) VALUES (?,?,?,'draft')`, projectID, organizationID, "Delivery runtime integration"); err != nil {
		_, _ = db.ExecContext(ctx, `DELETE FROM organizations WHERE id=?`, organizationID)
		t.Fatalf("create project fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM projects WHERE organization_id=? AND id=?`, organizationID, projectID)
		_, _ = db.ExecContext(ctx, `DELETE FROM organizations WHERE id=?`, organizationID)
	})
	actor := contract.ActorContext{OrganizationID: organizationID, Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "integration-user"}, Scopes: contract.ScopesFromStrings([]string{string(ScopeRead), string(ScopeWrite), string(ScopeApprove), string(ScopeExecute)})}
	repository := MySQLRepository{DB: db}
	counter := 0
	service := Service{Repository: repository, Projects: integrationProjectAuthorizer{}, NewID: func(prefix string) (string, error) {
		counter++
		return fmt.Sprintf("%s_%s_%d", prefix, suffix, counter), nil
	}, Now: func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }}

	intent, configuration := readyOceanRuntimeInputs(t, 2)
	intent.IntentID = "intent-" + suffix
	intent.CanonicalHash = ""
	intent, err = FinalizeDeliveryIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConfigurationID = "configuration-" + suffix
	configuration.Intent = IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash}
	configuration.CanonicalHash = ""
	configuration, err = FinalizePlatformConfiguration(configuration)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := service.CreatePlan(ctx, actor, projectID, CreatePlanRequest{Intent: &intent, PlatformConfiguration: &configuration})
	if err != nil {
		t.Fatalf("create v2 plan: %v", err)
	}
	legacyPlanID := "legacy-" + suffix
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_approvals WHERE organization_id=? AND project_id=? AND plan_id IN (?,?)`, organizationID, projectID, plan.ID, legacyPlanID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_change_sets WHERE organization_id=? AND project_id=? AND plan_id IN (?,?)`, organizationID, projectID, plan.ID, legacyPlanID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_plan_versions WHERE organization_id=? AND project_id=? AND plan_id IN (?,?)`, organizationID, projectID, plan.ID, legacyPlanID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_platform_configurations WHERE organization_id=? AND project_id=? AND configuration_id=?`, organizationID, projectID, configuration.ConfigurationID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_intents WHERE organization_id=? AND project_id=? AND intent_id=?`, organizationID, projectID, intent.IntentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_plans WHERE organization_id=? AND project_id=? AND id IN (?,?)`, organizationID, projectID, plan.ID, legacyPlanID)
	})

	loaded, err := service.GetPlan(ctx, actor, projectID, plan.ID)
	if err != nil {
		t.Fatalf("reload v2 plan: %v", err)
	}
	if loaded.CurrentVersion.CanonicalHash != configuration.CanonicalHash || loaded.CurrentVersion.PlatformConfiguration.CanonicalHash != configuration.CanonicalHash || len(loaded.CurrentVersion.PlatformConfiguration.Payload.OceanEngine.Promotions) != 2 {
		t.Fatalf("v2 round trip changed immutable content: %#v", loaded.CurrentVersion)
	}
	duplicateIntent := intent
	duplicateIntent.IntentID = "intent-duplicate-" + suffix
	duplicateIntent.CanonicalHash = ""
	duplicateIntent, err = FinalizeDeliveryIntent(duplicateIntent)
	if err != nil {
		t.Fatal(err)
	}
	if duplicateIntent.CanonicalHash != intent.CanonicalHash {
		t.Fatal("intent identity leaked into the canonical payload hash")
	}
	duplicateIntentJSON, err := jsonMarshal(duplicateIntent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO delivery_intents (organization_id,project_id,intent_id,version_number,schema_version,canonical_hash,hash_algorithm,intent_json,created_by,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, organizationID, projectID, duplicateIntent.IntentID, duplicateIntent.VersionNumber, duplicateIntent.SchemaVersion, duplicateIntent.CanonicalHash, duplicateIntent.HashAlgorithm, duplicateIntentJSON, actor.Principal.ID, service.now()); err != nil {
		t.Fatalf("persist equal intent payload under a distinct identity: %v", err)
	}
	duplicateConfiguration := configuration
	duplicateConfiguration.ConfigurationID = "configuration-duplicate-" + suffix
	duplicateConfiguration.CanonicalHash = ""
	duplicateConfiguration, err = FinalizePlatformConfiguration(duplicateConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	if duplicateConfiguration.CanonicalHash != configuration.CanonicalHash {
		t.Fatal("configuration identity leaked into the canonical payload hash")
	}
	duplicateConfigurationJSON, err := jsonMarshal(duplicateConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO delivery_platform_configurations (organization_id,project_id,configuration_id,version_number,schema_version,platform,profile_version,intent_id,intent_version,intent_canonical_hash,canonical_hash,hash_algorithm,configuration_json,created_by,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, organizationID, projectID, duplicateConfiguration.ConfigurationID, duplicateConfiguration.VersionNumber, duplicateConfiguration.SchemaVersion, duplicateConfiguration.Platform, duplicateConfiguration.ProfileVersion, duplicateConfiguration.Intent.IntentID, duplicateConfiguration.Intent.VersionNumber, duplicateConfiguration.Intent.CanonicalHash, duplicateConfiguration.CanonicalHash, duplicateConfiguration.HashAlgorithm, duplicateConfigurationJSON, actor.Principal.ID, service.now()); err != nil {
		t.Fatalf("persist equal configuration payload under a distinct identity: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_platform_configurations WHERE organization_id=? AND project_id=? AND configuration_id=?`, organizationID, projectID, duplicateConfiguration.ConfigurationID)
		_, _ = db.ExecContext(ctx, `DELETE FROM delivery_intents WHERE organization_id=? AND project_id=? AND intent_id=?`, organizationID, projectID, duplicateIntent.IntentID)
	})
	changeSet, err := service.CreateChangeSet(ctx, actor, projectID, plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(ctx, actor, projectID, changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Approve(ctx, actor, projectID, changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	if changeSet.Approval == nil || changeSet.Approval.ConfigurationCanonicalHash != configuration.CanonicalHash || changeSet.Approval.IntentCanonicalHash != intent.CanonicalHash {
		t.Fatalf("persisted approval bindings = %#v", changeSet.Approval)
	}

	legacy, err := versionFromDraft(DeliveryPlan{ID: legacyPlanID, OrganizationID: organizationID, ProjectID: projectID, Platform: "ocean_engine_mock"}, 1, goldenDraft(), actor.Principal, service.now())
	if err != nil {
		t.Fatal(err)
	}
	legacyPayload, err := jsonMarshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO delivery_plans (id,organization_id,project_id,creative_package_id,creative_package_hash,creative_version_id,name,objective,budget_cents,start_at,end_at,status,version,platform,source,scenario,current_version,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, legacyPlanID, organizationID, projectID, "legacy-asset", "legacy-hash", "1", legacy.Name, legacy.Objective, legacy.Budget.TotalMinor, legacy.Schedule.StartAt, legacy.Schedule.EndAt, DeliveryPlanDraft, 1, "ocean_engine_mock", SourceMock, legacy.Scenario, 1, actor.Principal.ID, service.now(), service.now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO delivery_plan_versions (organization_id,project_id,plan_id,version_number,config_json,canonical_hash,payload_schema_version,source,scenario,created_by_kind,created_by_id,created_at) VALUES (?,?,?,?,?,?,NULL,?,?,?,?,?)`, organizationID, projectID, legacyPlanID, 1, legacyPayload, legacy.CanonicalHash, legacy.Source, legacy.Scenario, legacy.CreatedBy.Kind, legacy.CreatedBy.ID, legacy.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BackfillPlanCanonicalHashes(ctx, db); err != nil {
		t.Fatalf("first standard backfill: %v", err)
	}
	if updated, err := BackfillPlanCanonicalHashes(ctx, db); err != nil || updated != 0 {
		t.Fatalf("second standard backfill updated=%d err=%v", updated, err)
	}
	legacyLoaded, err := repository.GetPlan(ctx, organizationID, projectID, legacyPlanID)
	if err != nil {
		t.Fatalf("reload legacy plan: %v", err)
	}
	if !legacyLoaded.CurrentVersion.ReadOnly || legacyLoaded.CurrentVersion.RuntimeStatus != PlanRuntimeLegacyUnsupported || legacyLoaded.CurrentVersion.CanonicalHash != legacy.CanonicalHash {
		t.Fatalf("legacy row changed after migration/backfills: %#v", legacyLoaded.CurrentVersion)
	}
	reusedIntentVersion := cloneVersion(loaded.CurrentVersion)
	reusedIntentVersion.VersionNumber = 2
	reusedIntentVersion.CreatedAt = service.now().Add(time.Minute)
	configurationV2 := cloneJSONPointer(loaded.CurrentVersion.PlatformConfiguration)
	configurationV2.VersionNumber = 2
	configurationV2.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor--
	configurationV2.CanonicalHash = ""
	configurationV2Value, err := FinalizePlatformConfiguration(*configurationV2)
	if err != nil {
		t.Fatal(err)
	}
	reusedIntentVersion.PlatformConfiguration = &configurationV2Value
	reusedIntentVersion.CanonicalHash = configurationV2Value.CanonicalHash
	if _, err = repository.UpdatePlan(ctx, organizationID, projectID, plan.ID, 1, reusedIntentVersion); err != nil {
		t.Fatalf("reuse immutable intent in a later platform configuration version: %v", err)
	}
}

func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }

type integrationProjectAuthorizer struct{}

func (integrationProjectAuthorizer) RequireActiveContext(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID) (contract.ProjectContext, error) {
	return contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: projectID}, nil
}

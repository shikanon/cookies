package browserautomation

import (
	"os"
	"strings"
	"testing"
)

func TestControlPlaneMigrationFreezesSafetyPrimitives(t *testing.T) {
	payload, err := os.ReadFile("../../../migrations/platform/20260812130000_platform_computer_use_control_plane.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(payload)
	for _, required := range []string{"computer_use_session_leases", "active_lock_key", "fencing_token", "computer_use_kill_switches", "computer_use_final_confirmations", "token_digest", "computer_use_controlled_action_attempts", "uq_computer_use_attempt_confirmation", "result_unknown"} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration omitted %q", required)
		}
	}
	if strings.Contains(strings.ToLower(sql), "remote_write_enabled") {
		t.Fatal("Platform control-plane migration must not alter Delivery remote-write authority")
	}
}

func TestBrowserRpaRenameMigrationIsMetadataOnly(t *testing.T) {
	payload, err := os.ReadFile("../../../migrations/platform/20260819120000_platform_browser_rpa_rename.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(payload)
	renames := map[string]string{
		"computer_use_environments":               "browser_rpa_environments",
		"computer_use_browser_profiles":           "browser_rpa_browser_profiles",
		"computer_use_site_policies":              "browser_rpa_site_policies",
		"computer_use_kill_switches":              "browser_rpa_kill_switches",
		"computer_use_runs":                       "browser_rpa_runs",
		"computer_use_session_leases":             "browser_rpa_session_leases",
		"computer_use_run_steps":                  "browser_rpa_run_steps",
		"computer_use_events":                     "browser_rpa_events",
		"computer_use_evidence":                   "browser_rpa_evidence",
		"computer_use_final_confirmations":        "browser_rpa_final_confirmations",
		"computer_use_confirmation_events":        "browser_rpa_confirmation_events",
		"computer_use_controlled_action_attempts": "browser_rpa_controlled_action_attempts",
	}
	for from, to := range renames {
		if !strings.Contains(sql, from+" TO "+to) {
			t.Errorf("rename migration omitted %q -> %q", from, to)
		}
	}
	if !strings.Contains(sql, "ALTER TABLE browser_rpa_environments") || !strings.Contains(sql, "cdp_endpoint") {
		t.Error("rename migration must add the environment cdp_endpoint column")
	}
	lower := strings.ToLower(sql)
	for _, forbidden := range []string{"update ", "delete ", "insert "} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("rename migration mutates row contents with %q", strings.TrimSpace(forbidden))
		}
	}
}

func TestDeliveryBrowserRpaRunReferenceRenameIsMetadataOnly(t *testing.T) {
	payload, err := os.ReadFile("../../../migrations/delivery/20260819121000_delivery_browser_rpa_run_reference_rename.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(payload)
	for _, table := range []string{"delivery_controlled_executions", "delivery_executions", "delivery_platform_entity_mappings", "delivery_platform_entity_mapping_revisions"} {
		if !strings.Contains(sql, "ALTER TABLE "+table) || !strings.Contains(sql, "CHANGE COLUMN computer_use_run_id browser_rpa_run_id") {
			t.Errorf("delivery rename migration omitted the run-reference column rename on %q", table)
		}
	}
	lower := strings.ToLower(sql)
	for _, forbidden := range []string{"update ", "delete ", "insert "} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("delivery rename migration mutates row contents with %q", strings.TrimSpace(forbidden))
		}
	}
}

func TestExecutionDriverMigrationDefaultsHistoricalRunsToPlaywright(t *testing.T) {
	payload, err := os.ReadFile("../../../migrations/platform/20260831110000_browser_rpa_execution_driver.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(payload)
	if !strings.Contains(sql, "ADD COLUMN execution_driver") || !strings.Contains(sql, "DEFAULT 'playwright-rpa/edge/v3'") {
		t.Fatalf("execution driver migration does not preserve historical runs: %s", sql)
	}
}

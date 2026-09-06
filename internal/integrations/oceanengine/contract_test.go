package oceanengine

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestContractVocabulary(t *testing.T) {
	if PlatformCode != "ocean_engine" || WebAPISessionMode != "web_api" || SourceConnector != "connector" {
		t.Fatalf("unexpected connector vocabulary")
	}
	for _, kind := range []ObjectKind{ObjectAccount, ObjectProject, ObjectPromotion, ObjectMaterial} {
		if !kind.Valid() {
			t.Fatalf("object kind %q is invalid", kind)
		}
	}
}

func TestWebAPIContractFixtureRemainsBlockedAndRedacted(t *testing.T) {
	payload, err := os.ReadFile("fixtures/web-api-contract-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SecSDKVersion      string            `json:"secsdk_version"`
		Status             string            `json:"status"`
		WriteEnabled       bool              `json:"write_enabled"`
		EnablementBlockers []string          `json:"enablement_blockers"`
		Redaction          map[string]string `json:"redaction"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SecSDKVersion != SecSDKVersion || fixture.Status != "create_contracts_confirmed" || fixture.WriteEnabled || len(fixture.EnablementBlockers) != 3 {
		t.Fatalf("fixture=%#v", fixture)
	}
	if fixture.Redaction["account"] != "<ACCOUNT_ID>" || fixture.Redaction["cookie"] != "<COOKIE>" || fixture.Redaction["csrf_token"] != "<CSRF_TOKEN>" {
		t.Fatalf("fixture redaction=%#v", fixture.Redaction)
	}
	lower := strings.ToLower(string(payload))
	for _, secret := range []string{"x-secsdk-csrf-token\": \"", "cookie\": \"session"} {
		if strings.Contains(lower, secret) {
			t.Fatalf("fixture contains forbidden value %q", secret)
		}
	}
	shape, err := os.ReadFile("fixtures/web-api-request-shapes-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"cookies-api-probe-", "http://", "https://", "sign_url\": \"", "video_id\": \""} {
		if strings.Contains(strings.ToLower(string(shape)), forbidden) {
			t.Fatalf("request shape contains forbidden value %q", forbidden)
		}
	}
}

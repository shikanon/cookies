package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type compilerStub struct{}

func (compilerStub) CompilePrepareV3(context.Context, browserautomation.BrowserRpaRun, browserautomation.SitePolicy) (json.RawMessage, error) {
	return json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","steps":[]}`), nil
}

type sessionStub struct{ err error }

func (s sessionStub) Check(context.Context, browserautomation.BrowserRpaRun) error { return s.err }

func TestPrepareUsesConnectorSessionAndProducesSanitizedPreview(t *testing.T) {
	repository := browserautomation.NewMemoryRepository()
	run := browserautomation.BrowserRpaRun{ID: "run_1", OrganizationID: contract.OrganizationID("org_1"), ProjectID: contract.ProjectID("project_1"), AccountID: "123", PolicyID: "policy_1", ExecutionDriver: browserautomation.ExecutionDriverOceanEngineWebAPI}
	repository.PutSitePolicy(browserautomation.SitePolicy{ID: run.PolicyID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, Platform: browserautomation.PlatformOceanEngine, AccountID: run.AccountID, Version: 1})
	adapter := Adapter{Compiler: compilerStub{}, Policies: repository, Sessions: sessionStub{}}
	page, err := adapter.Prepare(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if page.BeforeFacts["execution_driver"] != string(browserautomation.ExecutionDriverOceanEngineWebAPI) || page.Readback["compiled_input_sha256"] == "" || page.Readback["write_gate"] == "" || page.ScreenshotRef != "" {
		t.Fatalf("preview=%#v", page)
	}
}

func TestSubmitGateStopsBeforeContractCapture(t *testing.T) {
	run := browserautomation.BrowserRpaRun{AccountID: "123"}
	if err := (Adapter{}).CheckSubmit(run); !errors.Is(err, ErrWriteDisabled) {
		t.Fatalf("disabled error=%v", err)
	}
	adapter := Adapter{WriteEnabled: true, AccountAllowlist: []string{"123"}}
	if err := adapter.CheckSubmit(run); !errors.Is(err, ErrContractNotCaptured) {
		t.Fatalf("contract error=%v", err)
	}
}

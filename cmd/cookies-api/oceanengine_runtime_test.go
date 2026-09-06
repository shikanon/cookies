package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/connector"
)

type accountProbeSessionStore struct {
	value connector.OceanEngineAccountSession
}

func (s accountProbeSessionStore) GetAccountSession(context.Context, string, string) (connector.OceanEngineAccountSession, error) {
	return s.value, nil
}
func (accountProbeSessionStore) PutAccountSession(context.Context, connector.OceanEngineAccountSession, int64) (connector.OceanEngineAccountSession, error) {
	return connector.OceanEngineAccountSession{}, nil
}
func (accountProbeSessionStore) MarkAccountSessionVerified(context.Context, string, string, int64, time.Time) (connector.OceanEngineAccountSession, error) {
	return connector.OceanEngineAccountSession{}, nil
}

type accountProbeCipher struct{}

func (accountProbeCipher) Encrypt(value []byte) ([]byte, string, error) { return value, "test", nil }
func (accountProbeCipher) Decrypt(value []byte, _ string) ([]byte, error) {
	return append([]byte(nil), value...), nil
}

type accountExternalIDResolverStub struct{}

func (accountExternalIDResolverStub) ResolveExternalAccountID(context.Context, string, string, string) (string, error) {
	return "123", nil
}

func TestOceanEngineConnectorReaderUsesAccountSessionForProjectSync(t *testing.T) {
	factory := oceanEngineConnectorReaderFactory{
		accountSessions: accountProbeSessionStore{value: connector.OceanEngineAccountSession{
			OrganizationID: "org_1", AccountID: "oeacct_safe", Status: connector.AccountSessionReady,
			SessionCiphertext: []byte("session=organization"), SessionKeyVersion: "test", Version: 7,
		}},
		cipher:   accountProbeCipher{},
		baseURL:  "https://ad.oceanengine.com",
		accounts: accountExternalIDResolverStub{},
	}
	reader, cleanup, err := factory.Open(context.Background(), connector.SyncRequest{
		OrganizationID: "org_1",
		ProjectID:      "project_1",
		AccountRef:     "oeacct_safe",
	})
	if err != nil {
		t.Fatal(err)
	}
	client, ok := reader.(*oceanengine.Client)
	if !ok {
		t.Fatalf("reader type = %T", reader)
	}
	if client.Session.Cookies != "session=organization" || client.AdvertiserID != "123" {
		t.Fatalf("reader session/account = %#v/%q", client.Session, client.AdvertiserID)
	}
	cleanup()
	if client.Session.Cookies != "" {
		t.Fatal("reader cleanup retained the account session")
	}
}

func TestOceanEngineAccountProbeUsesOrganizationAccountSessionForProjectVerify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ad/api/account/info" || r.Header.Get("Cookie") != "session=organization" {
			t.Errorf("unexpected verification request: path=%q cookie=%q", r.URL.Path, r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	probe := oceanEngineAccountProbe{
		accountSessions: accountProbeSessionStore{value: connector.OceanEngineAccountSession{
			OrganizationID: "org_1", AccountID: "oeacct_safe", Status: connector.AccountSessionUnverified,
			SessionCiphertext: []byte("session=organization"), SessionKeyVersion: "test", Version: 7,
		}},
		cipher: accountProbeCipher{}, baseURL: server.URL, client: server.Client(),
	}
	version, err := probe.Verify(context.Background(), "org_1", "project_1", "oeacct_safe", "123")
	if err != nil {
		t.Fatal(err)
	}
	if version != 7 {
		t.Fatalf("session version=%d, want 7", version)
	}
}

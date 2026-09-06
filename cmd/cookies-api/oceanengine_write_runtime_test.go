package main

import (
	"context"
	"testing"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type externalAccountResolverStub struct{ accountID string }

func (s externalAccountResolverStub) ResolveAccountIDByExternalID(context.Context, string, string, string) (string, error) {
	return s.accountID, nil
}

type plaintextCipherStub struct{ plaintext []byte }

func (s *plaintextCipherStub) Encrypt([]byte) ([]byte, string, error) { return nil, "", nil }
func (s *plaintextCipherStub) Decrypt([]byte, string) ([]byte, error) { return s.plaintext, nil }

func TestOceanEngineConnectorWriterUsesReadySessionAndClearsPlaintext(t *testing.T) {
	cipher := &plaintextCipherStub{plaintext: []byte("session=secret")}
	factory := oceanEngineConnectorWriterFactory{
		accountSessions: accountProbeSessionStore{value: connector.OceanEngineAccountSession{OrganizationID: "org_1", AccountID: "oeacct_1", Status: connector.AccountSessionReady, SessionCiphertext: []byte("ciphertext"), SessionKeyVersion: "v1", Version: 4}},
		accounts:        externalAccountResolverStub{accountID: "oeacct_1"}, cipher: cipher, baseURL: "https://ad.oceanengine.com",
	}
	run := browserautomation.BrowserRpaRun{OrganizationID: contract.OrganizationID("org_1"), ProjectID: contract.ProjectID("project_1"), AccountID: "123"}
	writer, cleanup, err := factory.Open(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range cipher.plaintext {
		if value != 0 {
			t.Fatal("decrypted byte buffer was not cleared")
		}
	}
	if writer.Client.Session.Cookies == "" || writer.SessionVersion != 4 {
		t.Fatalf("writer=%#v", writer)
	}
	cleanup()
	if writer.Client.Session.Cookies != "" {
		t.Fatal("client session was not cleared")
	}
}

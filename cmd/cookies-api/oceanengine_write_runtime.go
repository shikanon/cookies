package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/browserautomation/webapi"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/systems/insights"
)

type oceanEngineExternalAccountResolver interface {
	ResolveAccountIDByExternalID(context.Context, string, string, string) (string, error)
}

type oceanEngineAccountSessionAuthMarker interface {
	MarkAccountSessionAuthRequired(context.Context, string, string, int64, time.Time) (connector.OceanEngineAccountSession, error)
}

type oceanEngineConnectorWriterFactory struct {
	accountSessions connector.AccountSessionStore
	authMarker      oceanEngineAccountSessionAuthMarker
	accounts        oceanEngineExternalAccountResolver
	cipher          insights.SecretCipher
	baseURL         string
	client          *http.Client
	tokenCache      *oceanengine.CSRFTokenCache
}

type oceanEngineConnectorWriter struct {
	Client         *oceanengine.WriteClient
	OrganizationID string
	AccountID      string
	SessionVersion int64
	factory        oceanEngineConnectorWriterFactory
}

func (f oceanEngineConnectorWriterFactory) Open(ctx context.Context, run browserautomation.BrowserRpaRun) (*oceanEngineConnectorWriter, func(), error) {
	if f.accountSessions == nil || f.accounts == nil || f.cipher == nil {
		return nil, nil, fmt.Errorf("Ocean Engine write session access is not configured")
	}
	accountID, err := f.accounts.ResolveAccountIDByExternalID(ctx, string(run.OrganizationID), string(run.ProjectID), run.AccountID)
	if err != nil {
		return nil, nil, browserautomation.ErrAccountMismatch
	}
	session, err := f.accountSessions.GetAccountSession(ctx, string(run.OrganizationID), accountID)
	if err != nil || session.Status != connector.AccountSessionReady {
		return nil, nil, browserautomation.ErrEnvironmentUnavailable
	}
	plaintext, err := f.cipher.Decrypt(session.SessionCiphertext, session.SessionKeyVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt Ocean Engine Connector session")
	}
	client, clientErr := oceanengine.NewWriteClient(f.baseURL, run.AccountID, session.Version, oceanengine.Session{Cookies: string(plaintext)}, f.client, f.tokenCache)
	for index := range plaintext {
		plaintext[index] = 0
	}
	if clientErr != nil {
		return nil, nil, clientErr
	}
	writer := &oceanEngineConnectorWriter{Client: client, OrganizationID: string(run.OrganizationID), AccountID: accountID, SessionVersion: session.Version, factory: f}
	cleanup := func() { client.Close() }
	return writer, cleanup, nil
}

func (w *oceanEngineConnectorWriter) MarkAuthRequired(ctx context.Context, now time.Time) error {
	if w == nil || w.factory.authMarker == nil {
		return fmt.Errorf("Ocean Engine session auth marker is not configured")
	}
	_, err := w.factory.authMarker.MarkAccountSessionAuthRequired(ctx, w.OrganizationID, w.AccountID, w.SessionVersion, now.UTC())
	return err
}

// OpenWebAPISession adapts the Connector writer factory to the Web API driver
// session contract: one decrypted write client plus its read client.
func (f oceanEngineConnectorWriterFactory) OpenSession(ctx context.Context, run browserautomation.BrowserRpaRun) (webapi.WriteSession, error) {
	writer, cleanup, err := f.Open(ctx, run)
	if err != nil {
		return webapi.WriteSession{}, err
	}
	reader, readerErr := oceanengine.NewClient(f.baseURL, run.AccountID, oceanengine.Session{Cookies: writer.Client.Session.Cookies}, f.client)
	if readerErr != nil {
		cleanup()
		return webapi.WriteSession{}, readerErr
	}
	reader.Delay = 0
	reader.MaxAttempts = 1
	return webapi.WriteSession{Writer: writer.Client, Reader: reader, Close: cleanup}, nil
}

// fileTemplateSource loads the account-calibrated create templates from the
// local git-ignored template file named by the configuration.
type fileTemplateSource struct {
	Path string
}

func (s fileTemplateSource) Load(context.Context) (webapi.CreateTemplates, error) {
	if s.Path == "" {
		return webapi.CreateTemplates{}, webapi.ErrTemplateNotConfigured
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return webapi.CreateTemplates{}, webapi.ErrTemplateNotConfigured
		}
		return webapi.CreateTemplates{}, err
	}
	var templates webapi.CreateTemplates
	if err := json.Unmarshal(data, &templates); err != nil {
		return webapi.CreateTemplates{}, fmt.Errorf("decode Ocean Engine Web API templates: %w", err)
	}
	return templates, nil
}

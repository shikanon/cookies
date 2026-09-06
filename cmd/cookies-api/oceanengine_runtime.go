package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/insights"
)

type oceanEngineSessionVerifier struct {
	baseURL string
	client  *http.Client
}

type oceanEngineWebAPISessionChecker struct {
	accountSessions connector.AccountSessionStore
	accounts        interface {
		ResolveAccountIDByExternalID(context.Context, string, string, string) (string, error)
	}
}

func (c oceanEngineWebAPISessionChecker) Check(ctx context.Context, run browserautomation.BrowserRpaRun) error {
	if c.accountSessions == nil || c.accounts == nil {
		return browserautomation.ErrEnvironmentUnavailable
	}
	accountID, err := c.accounts.ResolveAccountIDByExternalID(ctx, string(run.OrganizationID), string(run.ProjectID), run.AccountID)
	if err != nil {
		return browserautomation.ErrAccountMismatch
	}
	session, err := c.accountSessions.GetAccountSession(ctx, string(run.OrganizationID), accountID)
	if err != nil || session.Status != connector.AccountSessionReady {
		return browserautomation.ErrEnvironmentUnavailable
	}
	return nil
}

func (v oceanEngineSessionVerifier) VerifyOceanEngineSession(ctx context.Context, session []byte) error {
	if strings.TrimSpace(string(session)) == "" {
		return oceanengine.ErrSessionInvalid
	}
	client, err := oceanengine.NewSessionClient(v.baseURL, oceanengine.Session{Cookies: string(session)}, v.client)
	if err != nil {
		return err
	}
	client.Delay = 0
	_, err = client.GlobalInfo(ctx)
	if err != nil {
		return fmt.Errorf("ocean engine session verification failed")
	}
	return nil
}

var _ insights.OceanEngineSessionVerifier = oceanEngineSessionVerifier{}

type oceanEngineConnectorReaderFactory struct {
	sessions        insights.OceanEngineSessionRepository
	accountSessions connector.AccountSessionStore
	cipher          insights.SecretCipher
	baseURL         string
	client          *http.Client
	accounts        interface {
		ResolveExternalAccountID(context.Context, string, string, string) (string, error)
	}
}

func (f oceanEngineConnectorReaderFactory) Open(ctx context.Context, request connector.SyncRequest) (oceanengine.Reader, func(), error) {
	if f.cipher == nil {
		return nil, nil, fmt.Errorf("Ocean Engine session access is not configured")
	}
	var ciphertext []byte
	var keyVersion string
	if strings.HasPrefix(strings.TrimSpace(request.AccountRef), "oeacct_") {
		if f.accountSessions == nil {
			return nil, nil, fmt.Errorf("Ocean Engine account session access is not configured")
		}
		session, err := f.accountSessions.GetAccountSession(ctx, request.OrganizationID, request.AccountRef)
		if err != nil {
			return nil, nil, err
		}
		if session.Status != connector.AccountSessionReady {
			return nil, nil, fmt.Errorf("Ocean Engine account session is not ready")
		}
		ciphertext, keyVersion = session.SessionCiphertext, session.SessionKeyVersion
	} else {
		if f.sessions == nil {
			return nil, nil, fmt.Errorf("Ocean Engine Project session access is not configured")
		}
		session, err := f.sessions.GetProjectOceanEngineSession(ctx, contract.OrganizationID(request.OrganizationID), contract.ProjectID(request.ProjectID))
		if err != nil {
			return nil, nil, err
		}
		if session.Status != insights.OceanEngineSessionReady {
			return nil, nil, fmt.Errorf("Ocean Engine session is not ready")
		}
		ciphertext, keyVersion = session.SessionCiphertext, session.SessionKeyVersion
	}
	plaintext, err := f.cipher.Decrypt(ciphertext, keyVersion)
	if err != nil {
		return nil, nil, err
	}
	if f.accounts == nil {
		for index := range plaintext {
			plaintext[index] = 0
		}
		return nil, nil, fmt.Errorf("Ocean Engine account catalog is not configured")
	}
	externalID, resolveErr := f.accounts.ResolveExternalAccountID(ctx, request.OrganizationID, request.ProjectID, request.AccountRef)
	if resolveErr != nil {
		for index := range plaintext {
			plaintext[index] = 0
		}
		return nil, nil, resolveErr
	}
	client, err := oceanengine.NewClient(f.baseURL, externalID, oceanengine.Session{Cookies: string(plaintext)}, f.client)
	for index := range plaintext {
		plaintext[index] = 0
	}
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { client.Session.Cookies = ""; client.Session.CSRFToken = "" }
	return client, cleanup, nil
}

var _ connector.ReaderFactory = oceanEngineConnectorReaderFactory{}

type oceanEngineAccountProbe struct {
	accountSessions connector.AccountSessionStore
	cipher          insights.SecretCipher
	baseURL         string
	client          *http.Client
}

func (p oceanEngineAccountProbe) Verify(ctx context.Context, organizationID, projectID, accountID, externalID string) (int64, error) {
	if p.accountSessions == nil {
		return 0, fmt.Errorf("Ocean Engine account session access is not configured")
	}
	session, err := p.accountSessions.GetAccountSession(ctx, organizationID, accountID)
	if err != nil {
		return 0, err
	}
	if session.Status == connector.AccountSessionDisabled {
		return 0, fmt.Errorf("Ocean Engine account session is disabled")
	}
	plaintext, err := p.cipher.Decrypt(session.SessionCiphertext, session.SessionKeyVersion)
	if err != nil {
		return 0, err
	}
	client, err := oceanengine.NewClient(p.baseURL, externalID, oceanengine.Session{Cookies: string(plaintext)}, p.client)
	for index := range plaintext {
		plaintext[index] = 0
	}
	if err != nil {
		return 0, err
	}
	defer func() { client.Session.Cookies = ""; client.Session.CSRFToken = "" }()
	_, err = client.AccountInfo(ctx)
	if err != nil {
		var statusErr oceanengine.HTTPStatusError
		var redirectErr oceanengine.RedirectBlockedError
		if errors.Is(err, oceanengine.ErrSessionInvalid) || errors.As(err, &statusErr) && (statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden) || errors.As(err, &redirectErr) && (redirectErr.Reason == "authentication_required" || redirectErr.Reason == "account_context_required") {
			return 0, fmt.Errorf("%w", connector.ErrAccountSessionInvalid)
		}
		category := "upstream_error"
		switch {
		case errors.As(err, &statusErr):
			category = fmt.Sprintf("http_%d", statusErr.StatusCode)
		case errors.Is(err, context.DeadlineExceeded):
			category = "timeout"
		case errors.As(err, &redirectErr):
			category = "redirect_" + redirectErr.Reason
		case errors.Is(err, oceanengine.ErrForbiddenEndpoint):
			category = "forbidden_redirect"
		case strings.Contains(err.Error(), "decode Ocean Engine response"):
			category = "invalid_response"
		}
		if redirectErr.PathShape != "" {
			log.Printf("Ocean Engine account verification failed: category=%s path_shape=%s", category, redirectErr.PathShape)
		} else {
			log.Printf("Ocean Engine account verification failed: category=%s", category)
		}
		return 0, fmt.Errorf("Ocean Engine account verification failed")
	}
	return session.Version, nil
}

var _ connector.AccountProbe = oceanEngineAccountProbe{}

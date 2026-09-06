package oceanengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SecSDKVersion  = "1.2.22"
	writeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0"

	ProjectCreatePath      = "/superior/api/v2/project/create"
	ProjectListPath        = "/superior/api/v2/project/list"
	PromotionCreatePath    = "/superior/api/v2/promotion/create_promotion"
	PromotionListPath      = "/superior/api/ad/promotion/list"
	CheckProjectNamePath   = "/superior/api/agw/project/check_project_name"
	CheckPromotionNamePath = "/superior/api/agw/ad/check_promotion_name"
)

var (
	ErrCSRFTokenInvalid = errors.New("Ocean Engine CSRF token is invalid")
	ErrWriteForbidden   = errors.New("Ocean Engine write endpoint is forbidden")
	ErrResultUnknown    = errors.New("Ocean Engine write result is unknown")
	ErrAuthRequired     = errors.New("Ocean Engine authentication is required")
)

var writeEndpoints = map[Endpoint]struct{}{
	{Method: http.MethodPost, Path: ProjectCreatePath}:   {},
	{Method: http.MethodPost, Path: PromotionCreatePath}: {},
}

type csrfCacheKey struct {
	host           string
	advertiserID   string
	sessionVersion int64
}

type csrfCacheValue struct {
	token     string
	expiresAt time.Time
}

// CSRFTokenCache stores Secsdk tokens in process memory only.
type CSRFTokenCache struct {
	mu     sync.Mutex
	values map[csrfCacheKey]csrfCacheValue
}

func NewCSRFTokenCache() *CSRFTokenCache {
	return &CSRFTokenCache{values: make(map[csrfCacheKey]csrfCacheValue)}
}

type WriteClient struct {
	BaseURL        *url.URL
	HTTPClient     *http.Client
	Session        Session
	AdvertiserID   string
	SessionVersion int64
	UserAgent      string
	TokenCache     *CSRFTokenCache
	Now            func() time.Time
	// AllowSDKDowngrade permits the Secsdk 1.2.22 fallback only for a
	// separately authorized controlled probe. Production callers leave it false.
	AllowSDKDowngrade bool
	// ProbeSessionID carries the browser-generated UUID only for an isolated
	// field experiment. Production callers leave it empty.
	ProbeSessionID string
	// ProbeSigner adds the browser query signature only for an isolated field
	// experiment. Production callers leave it nil.
	ProbeSigner RequestSigner
	// ProbeBrowserHeaders adds the captured browser client-hint and fetch-metadata
	// headers only for an isolated field experiment. Production callers leave it
	// false.
	ProbeBrowserHeaders bool
	// ProbeReferer overrides the Referer with the captured page URL only for an
	// isolated field experiment. Production callers leave it empty.
	ProbeReferer string
	// ProbeUserAgent overrides the fixed User-Agent only for an isolated field
	// experiment. Production callers leave it empty.
	ProbeUserAgent string
}

// Captured browser parity values from the sanitized 2026-08-31 HAR. They name
// no secret: they are the standard Edge fetch-metadata and client-hint headers.
const (
	browserUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36 Edg/153.0.0.0"
	browserAcceptLanguage = "zh-CN,zh-TW;q=0.9,zh;q=0.8,en;q=0.7,en-GB;q=0.6,en-US;q=0.5"
	browserSecCHUA        = `"Microsoft Edge";v="153", "Not_A Brand";v="8", "Chromium";v="153"`
)

type RequestSigner func(context.Context, *url.URL, []byte) (string, error)

type WriteResponse struct {
	StatusCode int
	Body       json.RawMessage
}

// CSRFDiagnostic reports only non-secret properties of a Secsdk HEAD response.
type CSRFDiagnostic struct {
	HTTPStatus    int
	HeaderPresent bool
	PartCount     int
	StatusZero    bool
	TokenPresent  bool
	Downgrade     bool
	MaxAgeValid   bool
}

func NewWriteClient(rawBaseURL, advertiserID string, sessionVersion int64, session Session, httpClient *http.Client, cache *CSRFTokenCache) (*WriteClient, error) {
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil || baseURL.Scheme != "https" && baseURL.Scheme != "http" || baseURL.Host == "" || strings.TrimSpace(advertiserID) == "" || sessionVersion < 1 || strings.TrimSpace(session.Cookies) == "" {
		return nil, fmt.Errorf("invalid Ocean Engine write client configuration")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	copyClient := *httpClient
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if cache == nil {
		cache = NewCSRFTokenCache()
	}
	if strings.TrimSpace(session.CSRFToken) == "" {
		session.CSRFToken = cookieValue(session.Cookies, "csrftoken")
	}
	return &WriteClient{
		BaseURL: baseURL, HTTPClient: &copyClient, Session: session, AdvertiserID: advertiserID,
		SessionVersion: sessionVersion, UserAgent: writeUserAgent, TokenCache: cache, Now: time.Now,
	}, nil
}

// Close removes plaintext session material from the client.
func (c *WriteClient) Close() {
	c.Session.Cookies = ""
	c.Session.CSRFToken = ""
	c.ProbeSessionID = ""
	c.ProbeSigner = nil
	c.ProbeBrowserHeaders = false
	c.ProbeReferer = ""
	c.ProbeUserAgent = ""
}

// SubmitJSON sends one protected POST. It never retries the write.
func (c *WriteClient) SubmitJSON(ctx context.Context, path string, payload any) (WriteResponse, error) {
	if _, ok := writeEndpoints[Endpoint{Method: http.MethodPost, Path: path}]; !ok {
		return WriteResponse{}, fmt.Errorf("%w: POST %s", ErrWriteForbidden, path)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WriteResponse{}, fmt.Errorf("encode Ocean Engine write request: %w", err)
	}
	token, err := c.csrfToken(ctx, path)
	if err != nil {
		return WriteResponse{}, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return WriteResponse{}, err
	}
	if c.ProbeSigner != nil {
		targetCopy := *req.URL
		// The browser calls addSignature before updateQueryWithAdvid. The signer
		// therefore receives the protected URL without the advertiser query.
		signQuery := targetCopy.Query()
		signQuery.Del("aadvid")
		targetCopy.RawQuery = signQuery.Encode()
		signature, signErr := c.ProbeSigner(ctx, &targetCopy, append([]byte(nil), body...))
		if signErr != nil || strings.TrimSpace(signature) == "" || len(signature) > 256 {
			return WriteResponse{}, fmt.Errorf("create Ocean Engine request signature")
		}
		query := req.URL.Query()
		query.Set("_signature", signature)
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-secsdk-csrf-token", token)
	if c.ProbeSessionID != "" {
		req.Header.Set("x-sessionid", c.ProbeSessionID)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return WriteResponse{}, fmt.Errorf("%w: write transport failed", ErrResultUnknown)
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return WriteResponse{}, fmt.Errorf("%w: write response could not be read", ErrResultUnknown)
	}
	result := WriteResponse{StatusCode: resp.StatusCode, Body: json.RawMessage(responseBody)}
	if authResponse(resp) {
		return result, ErrAuthRequired
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return result, fmt.Errorf("%w: HTTP %d", ErrResultUnknown, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, HTTPStatusError{StatusCode: resp.StatusCode}
	}
	return result, nil
}

// Prepare validates the Secsdk token contract without sending a write request.
func (c *WriteClient) Prepare(ctx context.Context, path string) error {
	if _, ok := writeEndpoints[Endpoint{Method: http.MethodPost, Path: path}]; !ok {
		return fmt.Errorf("%w: POST %s", ErrWriteForbidden, path)
	}
	_, err := c.csrfToken(ctx, path)
	return err
}

// DiagnoseCSRF performs one read-only HEAD and never returns the token value.
func (c *WriteClient) DiagnoseCSRF(ctx context.Context, path string) (CSRFDiagnostic, error) {
	if _, ok := writeEndpoints[Endpoint{Method: http.MethodPost, Path: path}]; !ok {
		return CSRFDiagnostic{}, fmt.Errorf("%w: POST %s", ErrWriteForbidden, path)
	}
	req, err := c.newCSRFRequest(ctx, path)
	if err != nil {
		return CSRFDiagnostic{}, err
	}
	req.Header.Set("x-secsdk-csrf-request", "1")
	req.Header.Set("x-secsdk-csrf-version", SecSDKVersion)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CSRFDiagnostic{}, fmt.Errorf("diagnose Ocean Engine CSRF token: transport failed")
	}
	defer resp.Body.Close()
	diagnostic := CSRFDiagnostic{HTTPStatus: resp.StatusCode}
	if authResponse(resp) {
		return diagnostic, ErrAuthRequired
	}
	if resp.StatusCode != http.StatusOK {
		return diagnostic, fmt.Errorf("%w: token HTTP %d", ErrCSRFTokenInvalid, resp.StatusCode)
	}
	header := resp.Header.Get("x-ware-csrf-token")
	diagnostic.HeaderPresent = header != ""
	parts := strings.Split(header, ",")
	diagnostic.PartCount = len(parts)
	if len(parts) >= 3 {
		diagnostic.StatusZero = strings.TrimSpace(parts[0]) == "0"
		token := strings.TrimSpace(parts[1])
		diagnostic.TokenPresent = token != ""
		diagnostic.Downgrade = token == "DOWNGRADE"
		maxAgeMS, parseErr := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		diagnostic.MaxAgeValid = parseErr == nil && maxAgeMS > 0 && maxAgeMS <= int64((24*time.Hour)/time.Millisecond)
	}
	if !diagnostic.HeaderPresent || !diagnostic.StatusZero || !diagnostic.TokenPresent || diagnostic.Downgrade || !diagnostic.MaxAgeValid {
		return diagnostic, ErrCSRFTokenInvalid
	}
	return diagnostic, nil
}

func (c *WriteClient) csrfToken(ctx context.Context, protectedPath string) (string, error) {
	now := c.Now().UTC()
	key := csrfCacheKey{host: c.BaseURL.Host, advertiserID: c.AdvertiserID, sessionVersion: c.SessionVersion}
	c.TokenCache.mu.Lock()
	if cached, ok := c.TokenCache.values[key]; ok && cached.token != "" && now.Before(cached.expiresAt) {
		c.TokenCache.mu.Unlock()
		return cached.token, nil
	}
	c.TokenCache.mu.Unlock()

	// Secsdk 1.2.22 uses the protected POST path when TOKEN_PATH is undefined.
	req, err := c.newCSRFRequest(ctx, protectedPath)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-secsdk-csrf-request", "1")
	req.Header.Set("x-secsdk-csrf-version", SecSDKVersion)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("fetch Ocean Engine CSRF token: %w", context.DeadlineExceeded)
		}
		return "", fmt.Errorf("fetch Ocean Engine CSRF token: transport failed")
	}
	defer resp.Body.Close()
	if authResponse(resp) {
		return "", ErrAuthRequired
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: token HTTP %d", ErrCSRFTokenInvalid, resp.StatusCode)
	}
	header := resp.Header.Get("x-ware-csrf-token")
	if header == "" && c.AllowSDKDowngrade {
		return "DOWNGRADE", nil
	}
	parts := strings.Split(header, ",")
	if len(parts) < 3 || parts[0] != "0" || strings.TrimSpace(parts[1]) == "" || parts[1] == "DOWNGRADE" {
		return "", ErrCSRFTokenInvalid
	}
	maxAgeMS, parseErr := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if parseErr != nil || maxAgeMS <= 0 || maxAgeMS > int64((24*time.Hour)/time.Millisecond) {
		return "", ErrCSRFTokenInvalid
	}
	value := csrfCacheValue{token: parts[1], expiresAt: now.Add(time.Duration(maxAgeMS) * time.Millisecond)}
	c.TokenCache.mu.Lock()
	c.TokenCache.values[key] = value
	c.TokenCache.mu.Unlock()
	return value.token, nil
}

func (c *WriteClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "?") {
		return nil, fmt.Errorf("invalid Ocean Engine endpoint path")
	}
	target := *c.BaseURL
	target.Path = path
	target.RawQuery = "aadvid=" + url.QueryEscape(c.AdvertiserID)
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Cookie", c.Session.Cookies)
	if c.Session.CSRFToken != "" {
		req.Header.Set("x-csrftoken", c.Session.CSRFToken)
	}
	req.Header.Set("Origin", c.BaseURL.Scheme+"://"+c.BaseURL.Host)
	req.Header.Set("Referer", c.BaseURL.Scheme+"://"+c.BaseURL.Host+"/superior/")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.ProbeBrowserHeaders {
		req.Header.Set("accept-language", browserAcceptLanguage)
		req.Header.Set("cache-control", "no-cache")
		req.Header.Set("pragma", "no-cache")
		req.Header.Set("priority", "u=1, i")
		req.Header.Set("sec-ch-ua", browserSecCHUA)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
		req.Header.Set("sec-fetch-dest", "empty")
		req.Header.Set("sec-fetch-mode", "cors")
		req.Header.Set("sec-fetch-site", "same-origin")
	}
	if c.ProbeReferer != "" {
		req.Header.Set("Referer", c.ProbeReferer)
	}
	if c.ProbeUserAgent != "" {
		req.Header.Set("User-Agent", c.ProbeUserAgent)
	}
	return req, nil
}

func (c *WriteClient) newCSRFRequest(ctx context.Context, path string) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "?") {
		return nil, fmt.Errorf("invalid Ocean Engine endpoint path")
	}
	target := *c.BaseURL
	target.Path = path
	target.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Cookie", c.Session.Cookies)
	req.Header.Set("Origin", c.BaseURL.Scheme+"://"+c.BaseURL.Host)
	req.Header.Set("Referer", c.BaseURL.Scheme+"://"+c.BaseURL.Host+"/superior/")
	req.Header.Set("User-Agent", c.UserAgent)
	return req, nil
}

func authResponse(resp *http.Response) bool {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return false
	}
	location := strings.ToLower(resp.Header.Get("Location"))
	return strings.Contains(location, "login") || strings.Contains(location, "sso") || strings.Contains(location, "auth")
}

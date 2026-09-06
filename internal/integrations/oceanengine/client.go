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
	"strings"
	"time"
)

var ErrForbiddenEndpoint = errors.New("oceanengine endpoint is not read-only")
var ErrSessionInvalid = errors.New("oceanengine session is invalid")

type HTTPStatusError struct {
	StatusCode int
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("oceanengine HTTP status %d", e.StatusCode)
}

type BusinessCodeError struct {
	Code int
}

func (e BusinessCodeError) Error() string {
	return fmt.Sprintf("%s: business code %d", ErrSessionInvalid.Error(), e.Code)
}

func (e BusinessCodeError) Unwrap() error {
	return ErrSessionInvalid
}

type RedirectBlockedError struct {
	Reason    string
	PathShape string
}

func (e RedirectBlockedError) Error() string {
	return "oceanengine redirect blocked: " + e.Reason
}

func (e RedirectBlockedError) Unwrap() error {
	return ErrForbiddenEndpoint
}

type Endpoint struct {
	Method string
	Path   string
}

var readOnlyEndpoints = map[Endpoint]struct{}{
	{http.MethodPost, ProjectListPath}:                                      {},
	{http.MethodPost, PromotionListPath}:                                    {},
	{http.MethodGet, "/superior/api/v2/project/detail"}:                     {},
	{http.MethodGet, "/superior/api/v2/ad/promotion/detail"}:                {},
	{http.MethodPost, "/ad/api/promotion/ads/list"}:                         {},
	{http.MethodGet, "/superior/api/ad/promotion/detail"}:                   {},
	{http.MethodGet, "/ad/api/promotion/ads/get_promotion_detail"}:          {},
	{http.MethodGet, "/ad/api/promotion/ads/detail"}:                        {},
	{http.MethodPost, "/ad/api/promotion/ads/attribute/list"}:               {},
	{http.MethodPost, "/report/api/tool/agw/statistics_sophonx/statQuery"}:  {},
	{http.MethodPost, "/ad/api/agw/statistics_sophonx/statQuery"}:           {},
	{http.MethodGet, "/ad/api/account/info"}:                                {},
	{http.MethodGet, "/superior/api/v2/account/info"}:                       {},
	{http.MethodGet, "/superior/api/v2/account/conf"}:                       {},
	{http.MethodGet, "/ad/api/account/conf"}:                                {},
	{http.MethodGet, "/api/ebp/ebp_info/get_global_info"}:                   {},
	{http.MethodPost, "/superior/api/v2/ad/getImageList"}:                   {},
	{http.MethodPost, "/superior/api/v2/creative/material/picture/sign"}:    {},
	{http.MethodGet, "/superior/api/v2/video/list"}:                         {},
	{http.MethodGet, "/superior/api/v2/creative/material/aweme_photo_list"}: {},
	{http.MethodPost, "/superior/api/v2/ad/product/clue_product_list"}:      {},
	{http.MethodGet, "/platform/api/v1/orange/third_part_list"}:             {},
	{http.MethodGet, "/superior/api/v2/ad/get_orange_landing_page"}:         {},
	{http.MethodPost, "/superior/api/v2/project/get_optimization_goal_v2"}:  {},
	{http.MethodGet, "/nbs/api/ads/brand/yuntu/query_brand_industry"}:       {},
	{http.MethodGet, "/superior/api/v2/agw/ad/brand"}:                       {},
	{http.MethodPost, "/superior/api/v2/ad/authorize/list"}:                 {},
	{http.MethodGet, CheckProjectNamePath}:                                  {},
	{http.MethodGet, CheckPromotionNamePath}:                                {},
	{http.MethodGet, "/superior/api/project"}:                               {},
}

type Session struct {
	Cookies   string
	CSRFToken string
}

type Client struct {
	BaseURL      *url.URL
	HTTPClient   *http.Client
	Session      Session
	UserAgent    string
	AdvertiserID string
	Delay        time.Duration
	MaxAttempts  int
}

func NewClient(rawBaseURL, advertiserID string, session Session, httpClient *http.Client) (*Client, error) {
	client, err := newClient(rawBaseURL, advertiserID, session, httpClient)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(advertiserID) == "" {
		return nil, fmt.Errorf("advertiser ID and in-memory cookie session are required")
	}
	return client, nil
}

// NewSessionClient creates a read-only client for session verification.
func NewSessionClient(rawBaseURL string, session Session, httpClient *http.Client) (*Client, error) {
	return newClient(rawBaseURL, "", session, httpClient)
}

func newClient(rawBaseURL, advertiserID string, session Session, httpClient *http.Client) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(rawBaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid Ocean Engine base URL")
	}
	if strings.TrimSpace(session.Cookies) == "" {
		return nil, fmt.Errorf("in-memory cookie session is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	clientCopy := *httpClient
	previousRedirect := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !strings.EqualFold(req.URL.Host, base.Host) {
			host := strings.ToLower(req.URL.Hostname())
			if strings.Contains(host, "sso.") || strings.Contains(host, "login.") {
				return RedirectBlockedError{Reason: "authentication_required", PathShape: safeRedirectPathShape(req.URL.Path)}
			}
			return RedirectBlockedError{Reason: "cross_host", PathShape: safeRedirectPathShape(req.URL.Path)}
		}
		redirectPath := req.URL.Path
		if redirectPath != "/" {
			redirectPath = strings.TrimSuffix(redirectPath, "/")
		}
		if _, ok := readOnlyEndpoints[Endpoint{req.Method, redirectPath}]; !ok {
			path := strings.ToLower(req.URL.Path)
			if strings.Contains(path, "login") || strings.Contains(path, "sso") || strings.Contains(path, "auth") {
				return RedirectBlockedError{Reason: "authentication_required", PathShape: safeRedirectPathShape(req.URL.Path)}
			}
			if path == "/brand" {
				return RedirectBlockedError{Reason: "account_context_required", PathShape: safeRedirectPathShape(req.URL.Path)}
			}
			if path == "/" || strings.HasSuffix(path, ".html") {
				return RedirectBlockedError{Reason: "application_page", PathShape: safeRedirectPathShape(req.URL.Path)}
			}
			if strings.Contains(path, "account") || strings.Contains(path, "advertiser") {
				return RedirectBlockedError{Reason: "unknown_account_path", PathShape: safeRedirectPathShape(req.URL.Path)}
			}
			return RedirectBlockedError{Reason: "unknown_path", PathShape: safeRedirectPathShape(req.URL.Path)}
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 5 {
			return fmt.Errorf("too many Ocean Engine redirects")
		}
		return nil
	}
	httpClient = &clientCopy
	if strings.TrimSpace(session.CSRFToken) == "" {
		session.CSRFToken = cookieValue(session.Cookies, "csrftoken")
	}
	return &Client{BaseURL: base, HTTPClient: httpClient, Session: session, AdvertiserID: advertiserID, UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0", Delay: 500 * time.Millisecond, MaxAttempts: 3}, nil
}

func safeRedirectPathShape(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if len(segment) > 48 {
			segments[index] = "*"
			continue
		}
		var shaped strings.Builder
		for _, char := range strings.ToLower(segment) {
			switch {
			case char >= 'a' && char <= 'z', char == '-', char == '_', char == '.':
				shaped.WriteRune(char)
			case char >= '0' && char <= '9':
				shaped.WriteByte('#')
			default:
				shaped.WriteByte('*')
			}
		}
		segments[index] = shaped.String()
	}
	return strings.Join(segments, "/")
}

func cookieValue(header, name string) string {
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// CheckProjectName runs the read-only project-name availability query the
// browser sends before a create. It is part of the read-only calibration
// surface only.
func (c *Client) CheckProjectName(ctx context.Context, projectName string) (map[string]any, error) {
	query := url.Values{}
	query.Set("projectName", projectName)
	return c.do(ctx, http.MethodGet, CheckProjectNamePath+"?"+query.Encode(), nil, "")
}

// CheckPromotionName runs the read-only promotion-name availability query the
// browser sends immediately before a promotion create.
func (c *Client) CheckPromotionName(ctx context.Context, promotionName string) (map[string]any, error) {
	query := url.Values{}
	query.Set("promotionName", promotionName)
	return c.do(ctx, http.MethodGet, CheckPromotionNamePath+"?"+query.Encode(), nil, "")
}

// GetProjects reads project rows by platform ID. This is the same read the
// browser issues after a create; the aggregate project/promotion list omits
// projects that have no promotions yet.
func (c *Client) GetProjects(ctx context.Context, projectIDs ...string) (map[string]any, error) {
	query := url.Values{}
	query.Set("project_ids", strings.Join(projectIDs, ","))
	query.Set("need_raw_campaign", "true")
	return c.do(ctx, http.MethodGet, "/superior/api/project?"+query.Encode(), nil, "")
}

// ProjectDetails reads the complete project state used by the Superior edit
// form. It is read-only and includes the platform-resolved external_action.
func (c *Client) ProjectDetails(ctx context.Context, projectIDs ...string) (map[string]any, error) {
	query := url.Values{}
	query.Set("project_ids", strings.Join(projectIDs, ","))
	query.Set("need_product_recognition", "true")
	query.Set("need_bind_product_material", "true")
	query.Set("need_keywords", "true")
	query.Set("need_ea_conversion_status", "true")
	query.Set("need_fill_history_blue_keywords_info", "true")
	return c.do(ctx, http.MethodGet, "/superior/api/v2/project/detail?"+query.Encode(), nil, "")
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string) (map[string]any, error) {
	endpointPath := path
	endpointQuery := ""
	if index := strings.IndexByte(path, '?'); index >= 0 {
		endpointPath, endpointQuery = path[:index], path[index+1:]
	}
	if _, ok := readOnlyEndpoints[Endpoint{method, endpointPath}]; !ok {
		return nil, fmt.Errorf("%w: %s %s", ErrForbiddenEndpoint, method, path)
	}
	bodyBytes := []byte(nil)
	if body != nil {
		var readErr error
		bodyBytes, readErr = io.ReadAll(io.LimitReader(body, 8<<20))
		if readErr != nil {
			return nil, readErr
		}
	}
	if c.Delay > 0 {
		timer := time.NewTimer(c.Delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	u := *c.BaseURL
	u.Path = strings.TrimRight(u.Path, "/") + endpointPath
	query := u.Query()
	if endpointQuery != "" {
		parsed, err := url.ParseQuery(endpointQuery)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint query: %w", err)
		}
		for key, values := range parsed {
			for _, value := range values {
				query.Add(key, value)
			}
		}
	}
	if c.AdvertiserID != "" {
		query.Set("aadvid", c.AdvertiserID)
	}
	u.RawQuery = query.Encode()
	attempts := c.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	origin := c.BaseURL.Scheme + "://" + c.BaseURL.Host
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Origin", origin)
		if c.AdvertiserID != "" {
			req.Header.Set("Referer", origin+"/promotion/promote-manage/ad?aadvid="+url.QueryEscape(c.AdvertiserID))
		} else if strings.EqualFold(c.BaseURL.Host, "business.oceanengine.com") {
			req.Header.Set("Referer", origin+"/")
		}
		req.Header.Set("Cookie", c.Session.Cookies)
		if c.Session.CSRFToken != "" {
			req.Header.Set("x-csrftoken", c.Session.CSRFToken)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, requestErr := c.HTTPClient.Do(req)
		if requestErr != nil {
			var redirectErr RedirectBlockedError
			if errors.As(requestErr, &redirectErr) {
				return nil, redirectErr
			}
			if attempt < attempts {
				if err := waitRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("oceanengine request failed: %w", requestErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt < attempts {
				if err := waitRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, HTTPStatusError{StatusCode: resp.StatusCode}
		}
		var payload map[string]any
		decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Ocean Engine response: %w", decodeErr)
		}
		if code, ok := payload["code"].(float64); ok && code != 0 {
			return nil, BusinessCodeError{Code: int(code)}
		}
		return payload, nil
	}
	return nil, fmt.Errorf("oceanengine request attempts exhausted")
}

func waitRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

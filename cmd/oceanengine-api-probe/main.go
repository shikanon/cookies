// oceanengine-api-probe performs one controlled project-create contract probe.
// It is disabled unless the normal write switch and account allowlist permit it.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/config"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/database"
	"github.com/shikanon/cookies/internal/systems/insights"
)

const (
	executeConfirmation          = "CREATE_PROJECT_ONCE"
	executePromotionConfirmation = "CREATE_PROMOTION_ONCE"
	acrawlerSourceMapURL         = "https://lf3-ad-platform.byteadverts.com/obj/ad-platform/platform/superior/assets/js/7834.07ab154d.js.map"
	acrawlerRuntimeSuffix        = "@byted/acrawler/dist/runtime.js"
	acrawlerRuntimeSHA256        = "f6cb1579cf756e234b20f4f38c59568a1e594c847d9c4102d2ba448da45014e3"
	acrawlerSourceMapMaxLen      = 16 << 20
	// browserParityUserAgent is the captured Edge user agent from the
	// sanitized 2026-08-31 HAR. It is probe-only.
	browserParityUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36 Edg/153.0.0.0"
)

type harDocument struct {
	Log struct {
		Entries []struct {
			Request struct {
				Method  string `json:"method"`
				URL     string `json:"url"`
				Cookies []struct {
					Name string `json:"name"`
				} `json:"cookies"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
				PostData struct {
					Text string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
		} `json:"entries"`
	} `json:"log"`
}

type accountBinding struct {
	organizationID string
	projectID      string
	accountID      string
	externalID     string
	session        connector.OceanEngineAccountSession
}

type probeSummary struct {
	Mode               string   `json:"mode"`
	NameSHA256         string   `json:"name_sha256,omitempty"`
	PayloadSHA256      string   `json:"payload_sha256,omitempty"`
	OptionalFields     string   `json:"optional_fields"`
	ErrorKind          string   `json:"error_kind,omitempty"`
	CSRFHTTPStatus     int      `json:"csrf_http_status,omitempty"`
	CSRFHeader         bool     `json:"csrf_header_present,omitempty"`
	CSRFPartCount      int      `json:"csrf_part_count,omitempty"`
	CSRFStatusZero     bool     `json:"csrf_status_zero,omitempty"`
	CSRFTokenPresent   bool     `json:"csrf_token_present,omitempty"`
	CSRFMaxAgeValid    bool     `json:"csrf_max_age_valid,omitempty"`
	CSRFDowngrade      bool     `json:"csrf_downgrade,omitempty"`
	SDKDowngradeUsed   bool     `json:"sdk_downgrade_used,omitempty"`
	ConnectorCookies   int      `json:"connector_cookie_name_count,omitempty"`
	CapturedCookies    int      `json:"captured_cookie_name_count,omitempty"`
	MissingCookies     []string `json:"missing_cookie_names,omitempty"`
	HTTPStatus         int      `json:"http_status,omitempty"`
	BusinessCode       int      `json:"business_code,omitempty"`
	ExactMatchCount    int      `json:"exact_match_count,omitempty"`
	ResponseIDFound    bool     `json:"response_id_found,omitempty"`
	QueryIDFound       bool     `json:"query_id_found,omitempty"`
	ResponseIDsMatch   bool     `json:"response_ids_match,omitempty"`
	SignaturePresent   bool     `json:"signature_present,omitempty"`
	SignatureLength    int      `json:"signature_length,omitempty"`
	ChainedProjectID   string   `json:"chained_project_id,omitempty"`
	ChainedProjectCode int      `json:"chained_project_code,omitempty"`
	Result             string   `json:"result"`
}

func main() {
	var harPath, accountDigest, confirmation, reconcileDigest, projectID string
	var execute, executePromotion, chainProject, csrfOnly, cookieSchema, signatureCheck, checkName, allowSDKDowngrade, withSessionID, withSignature, withInvalidSignature, withBrowserHeaders bool
	flag.StringVar(&harPath, "har", "", "path to the local HAR capture")
	flag.StringVar(&accountDigest, "account-sha256", "", "SHA-256 of the selected external account ID")
	flag.BoolVar(&execute, "execute", false, "send one project-create POST")
	flag.BoolVar(&executePromotion, "execute-promotion", false, "send one promotion-create POST after a chained project create")
	flag.BoolVar(&chainProject, "chain-project", false, "with execute-promotion: create the project in the same run and attach the promotion to it")
	flag.BoolVar(&csrfOnly, "csrf-only", false, "perform only the protected-path HEAD token request")
	flag.BoolVar(&cookieSchema, "cookie-schema", false, "compare Connector and captured Cookie names without values")
	flag.BoolVar(&signatureCheck, "signature-check", false, "generate signature metadata without a platform request")
	flag.BoolVar(&checkName, "check-name", false, "run only the read-only project-name availability query")
	flag.BoolVar(&allowSDKDowngrade, "allow-sdk-downgrade", false, "allow the observed Secsdk fallback for this one probe")
	flag.BoolVar(&withSessionID, "with-session-id", false, "add only a new browser-style UUID session ID to this one probe")
	flag.BoolVar(&withSignature, "with-signature", false, "add only a pinned acrawler signature to this one probe")
	flag.BoolVar(&withInvalidSignature, "with-invalid-signature", false, "add one fixed malformed signature to this one probe")
	flag.BoolVar(&withBrowserHeaders, "with-browser-headers", false, "add the captured browser header surface to this one probe")
	flag.StringVar(&projectID, "project-id", "", "platform project ID to attach the promotion to when chain-project is absent")
	flag.StringVar(&reconcileDigest, "reconcile-name-sha256", "", "query only and match an exact object-name digest")
	flag.StringVar(&confirmation, "confirm", "", "must equal the mode confirmation literal when executing")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := run(ctx, harPath, accountDigest, execute, executePromotion, chainProject, csrfOnly, cookieSchema, signatureCheck, checkName, allowSDKDowngrade, withSessionID, withSignature, withInvalidSignature, withBrowserHeaders, projectID, reconcileDigest, confirmation); err != nil {
		fmt.Fprintln(os.Stderr, "probe failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, harPath, accountDigest string, execute, executePromotion, chainProject, csrfOnly, cookieSchema, signatureCheck, checkName, allowSDKDowngrade, withSessionID, withSignature, withInvalidSignature, withBrowserHeaders bool, projectID, reconcileDigest, confirmation string) error {
	modeCount := 0
	for _, selected := range []bool{execute, executePromotion, csrfOnly, cookieSchema, signatureCheck, checkName, reconcileDigest != ""} {
		if selected {
			modeCount++
		}
	}
	if len(accountDigest) != 64 || modeCount > 1 {
		return fmt.Errorf("one valid probe mode and the account digest are required")
	}
	if chainProject && !executePromotion {
		return fmt.Errorf("chain-project is valid only with execute-promotion")
	}
	if executePromotion && !chainProject && strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("execute-promotion needs chain-project or an explicit project-id")
	}
	var payload map[string]any
	var name string
	var err error
	summary := probeSummary{OptionalFields: "omitted", Result: "ready"}
	if !csrfOnly && !cookieSchema && reconcileDigest == "" {
		if strings.TrimSpace(harPath) == "" {
			return fmt.Errorf("HAR path is required")
		}
		if executePromotion {
			payload, err = promotionPayload(harPath, time.Now().UTC())
		} else {
			payload, err = projectPayload(harPath, time.Now().UTC())
		}
		if err != nil {
			return err
		}
		name, _ = payload["name"].(string)
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return fmt.Errorf("encode probe payload: %w", encodeErr)
		}
		summary.NameSHA256 = digestString(name)
		summary.PayloadSHA256 = digestBytes(encoded)
	}
	if !execute && !executePromotion && !csrfOnly && !cookieSchema && !signatureCheck && !checkName && reconcileDigest == "" {
		summary.Mode = "dry_run"
		return writeSummary(summary)
	}
	if execute && confirmation != executeConfirmation {
		return fmt.Errorf("exact execution confirmation is required")
	}
	if executePromotion && confirmation != executePromotionConfirmation {
		return fmt.Errorf("exact execution confirmation is required")
	}
	if allowSDKDowngrade && !execute && !executePromotion {
		return fmt.Errorf("SDK downgrade is valid only for an executing controlled probe")
	}
	signaturePresent := withSignature || withInvalidSignature
	if withSignature && withInvalidSignature {
		return fmt.Errorf("choose either the pinned signature or the malformed signature probe")
	}
	if withSessionID && (!execute && !executePromotion || !allowSDKDowngrade) {
		return fmt.Errorf("session ID isolation requires an executing SDK downgrade probe")
	}
	if withSignature && (!execute && !executePromotion || !allowSDKDowngrade || (withSessionID && !withBrowserHeaders && execute)) {
		return fmt.Errorf("signature isolation requires an executing SDK downgrade probe without a session ID")
	}
	if withInvalidSignature && (!execute || !allowSDKDowngrade) {
		return fmt.Errorf("the malformed signature probe requires an executing SDK downgrade probe")
	}
	if withBrowserHeaders && (!execute && !executePromotion || !allowSDKDowngrade || !signaturePresent || !withSessionID) {
		return fmt.Errorf("browser header parity requires an executing signature probe with a session ID")
	}
	if executePromotion && (!allowSDKDowngrade || !withSignature || !withSessionID || !withBrowserHeaders) {
		return fmt.Errorf("promotion create requires the confirmed probe shape: downgrade, signature, session ID, and browser headers")
	}
	if signatureCheck {
		target, targetErr := capturedProjectTarget(harPath, accountDigest)
		if targetErr != nil {
			return targetErr
		}
		body, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return fmt.Errorf("encode signature-check payload")
		}
		signer := newACrawlerProbeSigner(&http.Client{Timeout: 20 * time.Second})
		signature, signErr := signer(ctx, target, body)
		clear(body)
		if signErr != nil {
			return signErr
		}
		summary.Mode = "signature_check"
		summary.OptionalFields = "_signature_only"
		summary.SignaturePresent = signature != ""
		summary.SignatureLength = len(signature)
		summary.Result = "generated"
		return writeSummary(summary)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if !cfg.OceanEngine.Enabled || execute && !cfg.OceanEngine.WebAPIWriteEnabled {
		return fmt.Errorf("required Ocean Engine switches are not enabled")
	}
	db, err := database.Open(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer db.Close()
	repository := connector.MySQLRepository{DB: db}
	binding, err := findAccount(ctx, repository, strings.ToLower(accountDigest))
	if err != nil {
		return err
	}
	if execute && !slices.Contains(cfg.OceanEngine.WebAPIWriteAccounts, binding.externalID) {
		return fmt.Errorf("selected account is not in the Web API write allowlist")
	}
	cipher, err := insights.NewAESGCMSecretCipher(cfg.OceanEngine.MasterKey, cfg.OceanEngine.MasterKeyVersion)
	if err != nil {
		return fmt.Errorf("configure Connector session decryption: %w", err)
	}
	plaintext, err := cipher.Decrypt(binding.session.SessionCiphertext, binding.session.SessionKeyVersion)
	if err != nil {
		return fmt.Errorf("decrypt Connector session")
	}
	defer clear(plaintext)
	if cookieSchema {
		captured, captureErr := capturedCookieNames(harPath)
		if captureErr != nil {
			return captureErr
		}
		connectorNames := cookieNames(string(plaintext))
		summary.Mode = "cookie_schema"
		summary.CapturedCookies = len(captured)
		summary.ConnectorCookies = len(connectorNames)
		for _, name := range captured {
			if !slices.Contains(connectorNames, name) {
				summary.MissingCookies = append(summary.MissingCookies, name)
			}
		}
		summary.Result = "compared"
		return writeSummary(summary)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	writer, err := oceanengine.NewWriteClient(cfg.OceanEngine.BaseURL, binding.externalID, binding.session.Version, oceanengine.Session{Cookies: string(plaintext)}, httpClient, nil)
	if err != nil {
		return err
	}
	defer writer.Close()
	writer.AllowSDKDowngrade = allowSDKDowngrade
	summary.SDKDowngradeUsed = allowSDKDowngrade
	if withSessionID {
		writer.ProbeSessionID, err = newUUIDv4()
		if err != nil {
			return err
		}
		summary.OptionalFields = "x-sessionid_only"
	}
	if withSignature {
		writer.ProbeSigner = newACrawlerProbeSigner(&http.Client{Timeout: 20 * time.Second})
		summary.OptionalFields = "_signature_only"
	}
	if withInvalidSignature {
		writer.ProbeSigner = func(context.Context, *url.URL, []byte) (string, error) {
			return "AAAAinvalidAAAAinvalidAAAAinvalidA", nil
		}
		summary.OptionalFields = "invalid_signature_only"
	}
	if withBrowserHeaders {
		cascadeID, uuidErr := newUUIDv4()
		if uuidErr != nil {
			return uuidErr
		}
		writer.ProbeBrowserHeaders = true
		writer.ProbeUserAgent = browserParityUserAgent
		writer.ProbeReferer = fmt.Sprintf(
			"%s/superior/create-project?aadvid=%s&is_create=1&campaign_type=1&search_pkg=false&fromPage=newProject&cascade_id=%s",
			strings.TrimRight(cfg.OceanEngine.BaseURL, "/"), binding.externalID, cascadeID)
		summary.OptionalFields = "browser_headers_full_parity"
	}
	reader, err := oceanengine.NewClient(cfg.OceanEngine.BaseURL, binding.externalID, oceanengine.Session{Cookies: string(plaintext)}, httpClient)
	if err != nil {
		return err
	}
	reader.Delay = 0
	reader.MaxAttempts = 1
	if csrfOnly {
		summary.Mode = "csrf_only"
		diagnostic, prepareErr := writer.DiagnoseCSRF(ctx, oceanengine.ProjectCreatePath)
		summary.CSRFHTTPStatus = diagnostic.HTTPStatus
		summary.CSRFHeader = diagnostic.HeaderPresent
		summary.CSRFPartCount = diagnostic.PartCount
		summary.CSRFStatusZero = diagnostic.StatusZero
		summary.CSRFTokenPresent = diagnostic.TokenPresent
		summary.CSRFMaxAgeValid = diagnostic.MaxAgeValid
		summary.CSRFDowngrade = diagnostic.Downgrade
		if prepareErr != nil {
			summary.ErrorKind = errorKind(prepareErr)
			summary.Result = "failed"
		} else {
			summary.Result = "ready"
		}
		return writeSummary(summary)
	}
	if reconcileDigest != "" {
		summary.Mode = "reconcile_only"
		query, queryErr := reader.ListPage(ctx, oceanengine.ListRequest{Start: "2026-09-01", End: "2026-09-02", Page: 1, Limit: 100})
		if queryErr != nil {
			summary.ErrorKind = errorKind(queryErr)
			summary.Result = "query_failed"
			return writeSummary(summary)
		}
		matches := exactNameDigestObjects(query, strings.ToLower(reconcileDigest))
		summary.ExactMatchCount = len(matches)
		summary.QueryIDFound = len(matches) == 1 && findNumericID(matches[0]) != ""
		switch len(matches) {
		case 0:
			summary.Result = "not_found"
		case 1:
			summary.Result = "confirmed"
		default:
			summary.Result = "manual_reconciliation"
		}
		return writeSummary(summary)
	}
	if checkName {
		summary.Mode = "check_name_only"
		summary.NameSHA256 = digestString(name)
		check, checkErr := reader.CheckProjectName(ctx, name)
		if checkErr != nil {
			summary.ErrorKind = errorKind(checkErr)
			summary.Result = "query_failed"
			return writeSummary(summary)
		}
		summary.BusinessCode = businessCode(check)
		if summary.BusinessCode == 0 {
			summary.Result = "available"
		} else {
			summary.Result = "platform_rejected"
		}
		return writeSummary(summary)
	}

	if executePromotion {
		return executePromotionFlow(ctx, writer, reader, harPath, name, payload, chainProject, projectID, &summary)
	}

	response, writeErr := func() (oceanengine.WriteResponse, error) {
		// The browser queries name availability for the exact name immediately
		// before it creates the project. Mirror that read-only step so the
		// experiment also tests any server-side precheck state.
		if _, checkErr := reader.CheckProjectName(ctx, name); checkErr != nil {
			return oceanengine.WriteResponse{}, fmt.Errorf("pre-create name check: %w", checkErr)
		}
		return writer.SubmitJSON(ctx, oceanengine.ProjectCreatePath, payload)
	}()
	summary.Mode = "execute"
	summary.HTTPStatus = response.StatusCode
	responseObject := decodeObject(response.Body)
	summary.BusinessCode = businessCode(responseObject)
	summary.ErrorKind = errorKind(writeErr)
	responseID := findNumericID(responseObject)
	summary.ResponseIDFound = responseID != ""

	// The browser confirms a create with a by-ID project read. The aggregate
	// project/promotion list omits projects that have no promotions yet, so it
	// is only the fallback reconciliation source.
	matches := []map[string]any{}
	var queryErr error
	queryID := ""
	if responseID != "" {
		detail, detailErr := reader.GetProjects(ctx, responseID)
		if detailErr != nil {
			queryErr = detailErr
		} else {
			matches = exactNameObjects(detail, name)
			if len(matches) == 1 {
				queryID = findNumericID(matches[0])
			}
		}
	}
	if len(matches) == 0 && queryErr == nil {
		query, listErr := reader.ListPage(ctx, oceanengine.ListRequest{
			Start: "2026-09-01", End: "2026-09-02", Page: 1, Limit: 100,
		})
		if listErr != nil {
			queryErr = listErr
		} else {
			matches = exactNameObjects(query, name)
			if len(matches) == 1 {
				queryID = findNumericID(matches[0])
			}
		}
	}
	summary.ExactMatchCount = len(matches)
	summary.QueryIDFound = queryID != ""
	summary.ResponseIDsMatch = responseID != "" && queryID != "" && responseID == queryID
	summary.Result = classifyExecutionResult(summary.BusinessCode, writeErr, queryErr, matches, responseID, queryID)
	if err := writeSummary(summary); err != nil {
		return err
	}
	if queryErr != nil {
		return fmt.Errorf("reconciliation query failed after the one write")
	}
	if writeErr != nil && summary.Result != "confirmed" {
		return fmt.Errorf("write did not produce a confirmed result")
	}
	if summary.Result == "deterministic_rejection" {
		return fmt.Errorf("platform rejected the write with business code %d", summary.BusinessCode)
	}
	return nil
}

// executePromotionFlow creates the promotion with the confirmed probe shape.
// With chain-project it first creates the project in the same run, mirroring
// the production submit order.
func executePromotionFlow(ctx context.Context, writer *oceanengine.WriteClient, reader *oceanengine.Client, harPath, name string, payload map[string]any, chainProject bool, projectID string, summary *probeSummary) error {
	targetProjectID := strings.TrimSpace(projectID)
	if chainProject {
		projectPayloadData, payloadErr := projectPayload(harPath, time.Now().UTC())
		if payloadErr != nil {
			return payloadErr
		}
		projectName, _ := projectPayloadData["name"].(string)
		if _, checkErr := reader.CheckProjectName(ctx, projectName); checkErr != nil {
			return fmt.Errorf("chained pre-create name check: %w", checkErr)
		}
		projectResponse, projectErr := writer.SubmitJSON(ctx, oceanengine.ProjectCreatePath, projectPayloadData)
		projectObject := decodeObject(projectResponse.Body)
		summary.ChainedProjectCode = businessCode(projectObject)
		if projectErr != nil && summary.ChainedProjectCode != 0 {
			return fmt.Errorf("chained project create failed: %w", projectErr)
		}
		targetProjectID = findNumericID(projectObject)
		summary.ChainedProjectID = targetProjectID
		if targetProjectID == "" {
			return fmt.Errorf("chained project create returned no platform ID")
		}
	}
	payload["project_id"] = targetProjectID
	if _, checkErr := reader.CheckPromotionName(ctx, name); checkErr != nil {
		return fmt.Errorf("pre-create promotion name check: %w", checkErr)
	}
	payload["check_hash"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	response, writeErr := writer.SubmitJSON(ctx, oceanengine.PromotionCreatePath, payload)
	responseObject := decodeObject(response.Body)
	summary.BusinessCode = businessCode(responseObject)
	summary.ErrorKind = errorKind(writeErr)
	promotionID := findNumericIDWithKeys(responseObject, "promotion_id", "id")
	summary.ResponseIDFound = promotionID != ""

	matches := []map[string]any{}
	var queryErr error
	queryID := ""
	if promotionID != "" {
		detail, detailErr := reader.PromotionMaterials(ctx, promotionID, false)
		if detailErr != nil {
			queryErr = detailErr
		} else {
			matches = exactNameObjects(detail, name)
			if len(matches) == 1 {
				queryID = findNumericIDWithKeys(matches[0], "promotion_id", "id")
			}
		}
	}
	if len(matches) == 0 && queryErr == nil {
		query, listErr := reader.ListPage(ctx, oceanengine.ListRequest{
			Start: "2026-09-01", End: "2026-09-02", Page: 1, Limit: 100,
		})
		if listErr != nil {
			queryErr = listErr
		} else {
			matches = exactNameObjects(query, name)
			if len(matches) == 1 {
				queryID = findNumericIDWithKeys(matches[0], "promotion_id", "id")
			}
		}
	}
	summary.ExactMatchCount = len(matches)
	summary.QueryIDFound = queryID != ""
	summary.ResponseIDsMatch = promotionID != "" && queryID != "" && promotionID == queryID
	summary.Result = classifyExecutionResult(summary.BusinessCode, writeErr, queryErr, matches, promotionID, queryID)
	if err := writeSummary(*summary); err != nil {
		return err
	}
	if queryErr != nil {
		return fmt.Errorf("reconciliation query failed after the promotion write")
	}
	if writeErr != nil && summary.Result != "confirmed" {
		return fmt.Errorf("promotion write did not produce a confirmed result")
	}
	if summary.Result == "deterministic_rejection" {
		return fmt.Errorf("platform rejected the promotion write with business code %d", summary.BusinessCode)
	}
	return nil
}

func capturedProjectTarget(harPath, accountDigest string) (*url.URL, error) {
	data, err := os.ReadFile(harPath)
	if err != nil {
		return nil, fmt.Errorf("read HAR: %w", err)
	}
	var document harDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode HAR: %w", err)
	}
	for _, entry := range document.Log.Entries {
		target, parseErr := url.Parse(entry.Request.URL)
		if parseErr != nil || entry.Request.Method != http.MethodPost || target.Host != "ad.oceanengine.com" || target.Path != oceanengine.ProjectCreatePath {
			continue
		}
		account := target.Query().Get("aadvid")
		if account == "" || digestString(account) != strings.ToLower(accountDigest) {
			return nil, fmt.Errorf("captured account does not match the approved digest")
		}
		query := target.Query()
		query.Del("_signature")
		query.Del("aadvid")
		target.RawQuery = query.Encode()
		target.Fragment = ""
		return target, nil
	}
	return nil, fmt.Errorf("captured project-create target is absent")
}

func newACrawlerProbeSigner(client *http.Client) oceanengine.RequestSigner {
	return func(ctx context.Context, target *url.URL, body []byte) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, acrawlerSourceMapURL, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("download pinned acrawler source map")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("download pinned acrawler source map: HTTP %d", resp.StatusCode)
		}
		encodedMap, err := io.ReadAll(io.LimitReader(resp.Body, acrawlerSourceMapMaxLen+1))
		if err != nil || len(encodedMap) > acrawlerSourceMapMaxLen {
			return "", fmt.Errorf("read pinned acrawler source map")
		}
		runtime, err := extractPinnedRuntime(encodedMap, acrawlerRuntimeSuffix, acrawlerRuntimeSHA256)
		if err != nil {
			return "", err
		}
		return runNodeSigner(ctx, runtime, target.String(), body)
	}
}

func extractPinnedRuntime(encodedMap []byte, sourceSuffix, expectedHash string) ([]byte, error) {
	var sourceMap struct {
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
	}
	if err := json.Unmarshal(encodedMap, &sourceMap); err != nil || len(sourceMap.Sources) != len(sourceMap.SourcesContent) {
		return nil, fmt.Errorf("decode pinned acrawler source map")
	}
	for index, name := range sourceMap.Sources {
		if strings.HasSuffix(strings.ReplaceAll(name, "+", "/"), sourceSuffix) {
			runtime := []byte(sourceMap.SourcesContent[index])
			if digestBytes(runtime) != expectedHash {
				return nil, fmt.Errorf("pinned acrawler runtime hash changed")
			}
			return runtime, nil
		}
	}
	return nil, fmt.Errorf("pinned acrawler runtime is absent")
}

func runNodeSigner(ctx context.Context, runtime []byte, target string, body []byte) (string, error) {
	if _, err := exec.LookPath("node"); err != nil {
		return "", fmt.Errorf("Node.js is required for the isolated signature probe")
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode signature body")
	}
	input, err := json.Marshal(map[string]any{
		"runtime": string(runtime),
		"nonce":   map[string]any{"url": target, "body": payload, "bodyVal2str": false},
	})
	if err != nil {
		return "", fmt.Errorf("encode signature input")
	}
	const script = `const fs=require("fs");const input=JSON.parse(fs.readFileSync(0,"utf8"));const holder={};const loaded=new Function("exports",input.runtime+"\n;return exports;")(holder);const value=loaded.sign(input.nonce);process.stdout.write(JSON.stringify({signature:value}));`
	command := exec.CommandContext(ctx, "node", "-e", script)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	clear(input)
	if err != nil {
		return "", fmt.Errorf("execute pinned acrawler runtime")
	}
	var result struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(output, &result); err != nil || strings.TrimSpace(result.Signature) == "" {
		return "", fmt.Errorf("decode acrawler signature")
	}
	clear(output)
	return result.Signature, nil
}

func newUUIDv4() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate probe session ID: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func classifyExecutionResult(businessCode int, writeErr, queryErr error, matches []map[string]any, responseID, queryID string) string {
	switch {
	case len(matches) > 1:
		return "manual_reconciliation"
	case len(matches) == 1 && queryID != "" && (responseID == "" || responseID == queryID):
		return "confirmed"
	case businessCode != 0 && writeErr == nil && queryErr == nil && len(matches) == 0:
		return "deterministic_rejection"
	case writeErr != nil || queryErr != nil || len(matches) == 0:
		return "result_unknown"
	default:
		return "contract_mismatch"
	}
}

func projectPayload(path string, now time.Time) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read HAR: %w", err)
	}
	var document harDocument
	if err = json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode HAR: %w", err)
	}
	for _, entry := range document.Log.Entries {
		parsed, parseErr := url.Parse(entry.Request.URL)
		if parseErr != nil || entry.Request.Method != http.MethodPost || parsed.Path != oceanengine.ProjectCreatePath {
			continue
		}
		var payload map[string]any
		if err = json.Unmarshal([]byte(entry.Request.PostData.Text), &payload); err != nil {
			return nil, fmt.Errorf("decode captured project request: %w", err)
		}
		stamp := now.Format("20060102T150405Z")
		payload["name"] = "cookies-api-probe-" + stamp + "-project"
		// Both captures send the schedule as epoch-second strings in UTC+8:
		// start at midnight of the first day, end at 23:59:59 of the last day.
		// A literal "2026-09-01" string is a contract violation.
		scheduleZone := time.FixedZone("UTC8", 8*60*60)
		payload["start_time"] = strconv.FormatInt(time.Date(2026, 9, 1, 0, 0, 0, 0, scheduleZone).Unix(), 10)
		payload["end_time"] = strconv.FormatInt(time.Date(2026, 9, 2, 23, 59, 59, 0, scheduleZone).Unix(), 10)
		return payload, nil
	}
	return nil, fmt.Errorf("captured project-create request was not found")
}

func promotionPayload(path string, now time.Time) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read HAR: %w", err)
	}
	var document harDocument
	if err = json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode HAR: %w", err)
	}
	for _, entry := range document.Log.Entries {
		parsed, parseErr := url.Parse(entry.Request.URL)
		if parseErr != nil || entry.Request.Method != http.MethodPost || parsed.Path != oceanengine.PromotionCreatePath {
			continue
		}
		var payload map[string]any
		if err = json.Unmarshal([]byte(entry.Request.PostData.Text), &payload); err != nil {
			return nil, fmt.Errorf("decode captured promotion request: %w", err)
		}
		stamp := now.Format("20060102T150405Z")
		payload["name"] = "cookies-api-probe-" + stamp + "-promotion"
		// The capture shows the check hash as a client millisecond timestamp
		// taken between the name check and the create. The flow refreshes it
		// right before the write.
		payload["check_hash"] = strconv.FormatInt(now.UnixMilli(), 10)
		return payload, nil
	}
	return nil, fmt.Errorf("captured promotion-create request was not found")
}

func capturedCookieNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read HAR: %w", err)
	}
	var document harDocument
	if err = json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode HAR: %w", err)
	}
	for _, entry := range document.Log.Entries {
		parsed, parseErr := url.Parse(entry.Request.URL)
		if parseErr != nil || entry.Request.Method != http.MethodPost || parsed.Path != oceanengine.ProjectCreatePath {
			continue
		}
		var names []string
		for _, cookie := range entry.Request.Cookies {
			if cookie.Name != "" {
				names = append(names, strings.ToLower(cookie.Name))
			}
		}
		if len(names) == 0 {
			for _, header := range entry.Request.Headers {
				if !strings.EqualFold(header.Name, "cookie") {
					continue
				}
				names = cookieNames(header.Value)
				break
			}
		}
		slices.Sort(names)
		return slices.Compact(names), nil
	}
	return nil, fmt.Errorf("captured project-create request was not found")
}

func cookieNames(header string) []string {
	var names []string
	for _, part := range strings.Split(header, ";") {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			names = append(names, strings.ToLower(strings.TrimSpace(name)))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func findAccount(ctx context.Context, repository connector.MySQLRepository, digest string) (accountBinding, error) {
	sessions, err := repository.ListReadyAccountSessions(ctx, 1000)
	if err != nil {
		return accountBinding{}, fmt.Errorf("list ready Connector sessions: %w", err)
	}
	var match *accountBinding
	for _, item := range sessions {
		externalID, resolveErr := repository.ResolveAnyExternalAccountID(ctx, item.OrganizationID, item.ProjectID, item.AccountID)
		if resolveErr != nil || digestString(externalID) != digest {
			continue
		}
		fullSession, getErr := repository.GetAccountSession(ctx, item.OrganizationID, item.AccountID)
		if getErr != nil {
			return accountBinding{}, fmt.Errorf("load Connector session: %w", getErr)
		}
		candidate := accountBinding{organizationID: item.OrganizationID, projectID: item.ProjectID, accountID: item.AccountID, externalID: externalID, session: fullSession}
		if match != nil && match.accountID != candidate.accountID {
			return accountBinding{}, fmt.Errorf("account digest matched multiple Connector accounts")
		}
		match = &candidate
	}
	if match == nil {
		return accountBinding{}, fmt.Errorf("selected Edge account has no ready Connector session")
	}
	return *match, nil
}

func decodeObject(data json.RawMessage) map[string]any {
	var value map[string]any
	_ = json.Unmarshal(data, &value)
	return value
}

func businessCode(value map[string]any) int {
	switch code := value["code"].(type) {
	case float64:
		return int(code)
	case json.Number:
		parsed, _ := code.Int64()
		return int(parsed)
	}
	return 0
}

func exactNameObjects(value any, name string) []map[string]any {
	var result []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			// The aggregate views name rows project_name and promotion_name.
			for _, key := range []string{"name", "project_name", "promotion_name"} {
				if typedName, ok := typed[key].(string); ok && typedName == name {
					result = append(result, typed)
					return
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func exactNameDigestObjects(value any, nameDigest string) []map[string]any {
	var result []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if name, ok := typed["name"].(string); ok && digestString(name) == nameDigest {
				result = append(result, typed)
				return
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func errorKind(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, oceanengine.ErrCSRFTokenInvalid):
		return "csrf_token_invalid"
	case errors.Is(err, oceanengine.ErrAuthRequired):
		return "auth_required"
	case errors.Is(err, oceanengine.ErrResultUnknown):
		return "transport_unknown"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		var status oceanengine.HTTPStatusError
		if errors.As(err, &status) {
			return fmt.Sprintf("http_%d", status.StatusCode)
		}
		return "request_failed"
	}
}

func findNumericID(value any) string {
	return findNumericIDWithKeys(value, "project_id", "campaign_id", "id")
}

func findNumericIDWithKeys(value any, preferred ...string) string {
	if object, ok := value.(map[string]any); ok {
		for _, key := range preferred {
			if found := numericString(object[key]); found != "" {
				return found
			}
		}
		for _, child := range object {
			if found := findNumericIDWithKeys(child, preferred...); found != "" {
				return found
			}
		}
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			if found := findNumericIDWithKeys(child, preferred...); found != "" {
				return found
			}
		}
	}
	return ""
}

func numericString(value any) string {
	switch typed := value.(type) {
	case string:
		if typed != "" && strings.IndexFunc(typed, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return typed
		}
	case float64:
		if typed > 0 && typed == float64(int64(typed)) {
			return fmt.Sprintf("%.0f", typed)
		}
	case json.Number:
		return string(typed)
	}
	return ""
}

func digestString(value string) string { return digestBytes([]byte(value)) }

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func writeSummary(summary probeSummary) error {
	encoded, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

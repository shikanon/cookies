package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/ids"
)

const MaxDocumentBytes int64 = 10 * 1024 * 1024
const maxExtractedBytes int64 = 20 * 1024 * 1024

var ErrInvalidDocument = errors.New("invalid knowledge document")
var ErrExternalConfirmationRequired = errors.New("external research confirmation is required")
var ErrExternalRunnerUnavailable = errors.New("external research runner is not configured")
var ErrInvalidResearchRequest = errors.New("invalid research request")

type ProjectReader interface {
	GetContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
}

type Service struct {
	DB                *sql.DB
	Projects          ProjectReader
	Blobs             assets.BlobStore
	Scanner           assets.ContentScanner
	AssetsBucket      string
	Runner            ExternalResearchRunner
	Scheduler         ResearchScheduler
	DocumentParser    DocumentParser
	DocumentScheduler DocumentParseScheduler
	Now               func() time.Time
	NewID             ids.Generator
}

type Document struct {
	ID                string                  `json:"id"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	Title             string                  `json:"title,omitempty"`
	SourceURI         string                  `json:"source_uri,omitempty"`
	SourceType        string                  `json:"source_type,omitempty"`
	ChunkCount        int                     `json:"chunk_count,omitempty"`
	ImportedBy        contract.Principal      `json:"imported_by,omitempty"`
	Filename          string                  `json:"filename"`
	MIMEType          string                  `json:"mime_type"`
	SizeBytes         int64                   `json:"size_bytes"`
	ContentSHA256     string                  `json:"content_sha256"`
	TextSHA256        string                  `json:"text_sha256"`
	ExtractedText     string                  `json:"extracted_text,omitempty"`
	Status            string                  `json:"status"`
	ParserCode        string                  `json:"parser_code,omitempty"`
	ParserVersion     string                  `json:"parser_version,omitempty"`
	ParseErrorCode    string                  `json:"parse_error_code,omitempty"`
	ParseErrorMessage string                  `json:"parse_error_message,omitempty"`
	ParseMetadata     json.RawMessage         `json:"parse_metadata,omitempty"`
	ParsedAt          *time.Time              `json:"parsed_at,omitempty"`
	CreatedBy         string                  `json:"created_by"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
	Blob              assets.ObjectLocation   `json:"-"`
}

type ResearchRequest struct {
	Mode            string                `json:"mode"`
	Category        string                `json:"category,omitempty"`
	Purpose         string                `json:"purpose,omitempty"`
	SourceRef       *contract.ResourceRef `json:"source_ref,omitempty"`
	Query           string                `json:"query"`
	DocumentIDs     []string              `json:"document_ids"`
	DisclosedFields []string              `json:"disclosed_fields"`
	Confirmed       bool                  `json:"confirmed"`
}

type ExternalResearchInput struct {
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	Mode           string                  `json:"mode"`
	Category       string                  `json:"category"`
	Purpose        string                  `json:"purpose"`
	Query          string                  `json:"query"`
	Documents      []ExternalDocument      `json:"documents"`
}

type ExternalDocument struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type ExternalResearchResult struct {
	Title            string                   `json:"title"`
	SourceURL        string                   `json:"source_url,omitempty"`
	Content          string                   `json:"content"`
	Citations        []string                 `json:"citations"`
	Sources          []ExternalResearchSource `json:"sources,omitempty"`
	ProviderCode     string                   `json:"provider_code,omitempty"`
	ModelVersion     string                   `json:"model_version,omitempty"`
	ProviderResponse string                   `json:"provider_response_id,omitempty"`
	Usage            *ResearchUsage           `json:"usage,omitempty"`
}

type ExternalResearchSource struct {
	SourceClass     string          `json:"source_class"`
	MediaType       string          `json:"media_type"`
	Title           string          `json:"title"`
	URL             string          `json:"url"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`
	StartIndex      int             `json:"start_index,omitempty"`
	EndIndex        int             `json:"end_index,omitempty"`
	ProviderLocator json.RawMessage `json:"provider_locator,omitempty"`
}

type ResearchUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type ExternalResearchRunner interface {
	Run(context.Context, ExternalResearchInput) ([]ExternalResearchResult, error)
}

type ResearchScheduler interface {
	Schedule(context.Context, ResearchRun) error
}

type DocumentParseScheduler interface {
	ScheduleDocumentParse(context.Context, Document) error
}

type ResearchRun struct {
	ID                 string                  `json:"id"`
	OrganizationID     contract.OrganizationID `json:"organization_id"`
	ProjectID          contract.ProjectID      `json:"project_id"`
	Mode               string                  `json:"mode"`
	Category           string                  `json:"category"`
	Purpose            string                  `json:"purpose"`
	SourceRef          *contract.ResourceRef   `json:"source_ref,omitempty"`
	Query              string                  `json:"query"`
	DocumentIDs        []string                `json:"document_ids"`
	DisclosedFields    []string                `json:"disclosed_fields"`
	DisclosedChunkIDs  []string                `json:"disclosed_chunk_ids"`
	Status             string                  `json:"status"`
	ConfirmedBy        string                  `json:"confirmed_by"`
	ConfirmedAt        time.Time               `json:"confirmed_at"`
	ErrorCode          string                  `json:"error_code,omitempty"`
	ErrorMessage       string                  `json:"error_message,omitempty"`
	ProviderCode       string                  `json:"provider_code,omitempty"`
	ModelVersion       string                  `json:"model_version,omitempty"`
	ProviderResponseID string                  `json:"provider_response_id,omitempty"`
	Usage              *ResearchUsage          `json:"usage,omitempty"`
	Artifacts          []ResearchArtifact      `json:"artifacts"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

type ResearchArtifact struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	ResearchRunID  string                  `json:"research_run_id"`
	SourceType     string                  `json:"source_type"`
	Category       string                  `json:"category"`
	Title          string                  `json:"title"`
	SourceURL      string                  `json:"source_url,omitempty"`
	Content        string                  `json:"content"`
	Citations      []string                `json:"citations"`
	Sources        []ResearchSource        `json:"sources"`
	ContentHash    string                  `json:"content_hash"`
	CreatedAt      time.Time               `json:"created_at"`
}

type ResearchSource struct {
	ID                 string                  `json:"id"`
	OrganizationID     contract.OrganizationID `json:"organization_id"`
	ProjectID          contract.ProjectID      `json:"project_id"`
	ResearchRunID      string                  `json:"research_run_id"`
	SourceClass        string                  `json:"source_class"`
	MediaType          string                  `json:"media_type"`
	Title              string                  `json:"title"`
	URL                string                  `json:"url"`
	CanonicalURL       string                  `json:"canonical_url"`
	Domain             string                  `json:"domain"`
	PublishedAt        *time.Time              `json:"published_at,omitempty"`
	RetrievedAt        time.Time               `json:"retrieved_at"`
	VerificationStatus string                  `json:"verification_status"`
	ContentHash        string                  `json:"content_hash"`
	StartIndex         int                     `json:"start_index"`
	EndIndex           int                     `json:"end_index"`
	SupportLevel       string                  `json:"support_level"`
	ProviderLocator    json.RawMessage         `json:"provider_locator,omitempty"`
}

type Reference struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	ContentHash string   `json:"content_hash"`
	Citations   []string `json:"citations,omitempty"`
}

func (s Service) CreateDocument(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, filename, declaredMIME string, source io.Reader, size int64) (Document, error) {
	if s.DB == nil || s.Projects == nil || s.Blobs == nil || s.Scanner == nil {
		return Document{}, fmt.Errorf("knowledge service dependencies are incomplete")
	}
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return Document{}, err
	}
	filename = strings.TrimSpace(filepath.Base(filename))
	extension := strings.ToLower(filepath.Ext(filename))
	if filename == "" || len(filename) > 512 ||
		(extension != ".md" && extension != ".docx" && extension != ".pdf") ||
		size < 1 || size > MaxDocumentBytes {
		return Document{}, ErrInvalidDocument
	}
	content, err := io.ReadAll(io.LimitReader(source, MaxDocumentBytes+1))
	if err != nil || int64(len(content)) != size || int64(len(content)) > MaxDocumentBytes {
		return Document{}, ErrInvalidDocument
	}
	if err := s.Scanner.Scan(ctx, bytes.NewReader(content)); err != nil {
		return Document{}, err
	}
	if declaredMIME != "" && !allowedMIME(extension, declaredMIME) {
		return Document{}, ErrInvalidDocument
	}
	mimeType := defaultDocumentMIME(extension)
	asyncParse := extension != ".md" && s.DocumentParser != nil && s.DocumentScheduler != nil
	contentSum := sha256.Sum256(content)
	contentHash := hex.EncodeToString(contentSum[:])
	if asyncParse {
		existing, err := scanDocument(s.DB.QueryRowContext(ctx, documentSelect+`
			WHERE organization_id = ? AND project_id = ? AND content_sha256 = ? AND status = 'ready'
			ORDER BY parsed_at DESC, created_at DESC LIMIT 1`,
			actor.OrganizationID, projectID, contentHash,
		))
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Document{}, err
		}
	}
	extracted := ""
	if !asyncParse {
		var err error
		extracted, mimeType, err = extractDocument(extension, content)
		if err != nil {
			return Document{}, err
		}
	}
	id, err := s.newID("knowledgedoc")
	if err != nil {
		return Document{}, err
	}
	textSum := sha256.Sum256([]byte(extracted))
	now := s.now()
	key := fmt.Sprintf("knowledge/%s/%s/%s/source%s", actor.OrganizationID, projectID, id, extension)
	object, err := s.Blobs.Put(ctx, s.AssetsBucket, key, bytes.NewReader(content), size, mimeType)
	if err != nil {
		return Document{}, err
	}
	document := Document{
		ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		Title: filename, SourceType: "docs",
		Filename: filename, MIMEType: mimeType, SizeBytes: size,
		ContentSHA256: contentHash, TextSHA256: hex.EncodeToString(textSum[:]),
		ExtractedText: extracted, Status: "parse_queued", CreatedBy: actor.Principal.ID,
		CreatedAt: now, UpdatedAt: now, Blob: object.ObjectLocation,
	}
	if !asyncParse {
		document.Status = "ready"
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO platform_knowledge_documents
		(id, organization_id, project_id, title, source_uri, source_type, chunk_count,
		 filename, mime_type, size_bytes, content_sha256,
		 text_sha256, extracted_text, object_provider, object_bucket, object_key,
		 object_version_id, object_etag, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		document.ID, document.OrganizationID, document.ProjectID, document.Title, nil,
		document.SourceType, document.ChunkCount, document.Filename, document.MIMEType,
		document.SizeBytes, document.ContentSHA256, document.TextSHA256, document.ExtractedText,
		object.Provider, object.Bucket, object.Key, object.VersionID, object.ETag, document.Status,
		document.CreatedBy, document.CreatedAt, document.UpdatedAt)
	if err != nil {
		_ = s.Blobs.Delete(ctx, object.ObjectLocation)
		return Document{}, err
	}
	if asyncParse {
		if err := s.DocumentScheduler.ScheduleDocumentParse(ctx, document); err != nil {
			_, _ = s.DB.ExecContext(ctx, `DELETE FROM platform_knowledge_documents
				WHERE organization_id = ? AND project_id = ? AND id = ?`,
				document.OrganizationID, document.ProjectID, document.ID)
			_ = s.Blobs.Delete(ctx, object.ObjectLocation)
			return Document{}, err
		}
		return document, nil
	}
	parsed := ParsedDocument{
		Text: extracted, MIMEType: mimeType, ParserCode: "native", ParserVersion: "1",
		Metadata: json.RawMessage(`{}`),
	}
	chunks := chunksForParsedDocument(document, parsed)
	if len(chunks) == 0 {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM platform_knowledge_documents
			WHERE organization_id = ? AND project_id = ? AND id = ?`,
			document.OrganizationID, document.ProjectID, document.ID)
		_ = s.Blobs.Delete(ctx, object.ObjectLocation)
		return Document{}, ErrInvalidDocument
	}
	if err := s.persistParsedDocument(ctx, document, parsed, chunks); err != nil {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM platform_knowledge_documents
			WHERE organization_id = ? AND project_id = ? AND id = ?`,
			document.OrganizationID, document.ProjectID, document.ID)
		_ = s.Blobs.Delete(ctx, object.ObjectLocation)
		return Document{}, err
	}
	document.ChunkCount = len(chunks)
	document.ParserCode, document.ParserVersion = parsed.ParserCode, parsed.ParserVersion
	document.ParsedAt = &now
	return document, nil
}

func (s Service) ImportDocument(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request ImportDocumentRequest) (Document, error) {
	if err := request.Validate(); err != nil {
		return Document{}, err
	}
	filename := strings.TrimSpace(request.Title)
	if !strings.HasSuffix(strings.ToLower(filename), ".md") {
		filename += ".md"
	}
	content := []byte(request.Text)
	document, err := s.CreateDocument(ctx, actor, projectID, filename, "text/markdown", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return Document{}, err
	}
	document.Title = strings.TrimSpace(request.Title)
	document.SourceURI = strings.TrimSpace(request.SourceURI)
	document.SourceType = normalizedSourceType(request.SourceType)
	document.ChunkCount = max(document.ChunkCount, 1)
	document.ImportedBy = actor.Principal
	_, err = s.DB.ExecContext(ctx, `UPDATE platform_knowledge_documents
		SET title = ?, source_uri = ?, source_type = ?, chunk_count = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		document.Title, nullable(document.SourceURI), document.SourceType, document.ChunkCount,
		document.UpdatedAt, actor.OrganizationID, projectID, document.ID)
	if err != nil {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM platform_knowledge_documents
			WHERE organization_id = ? AND project_id = ? AND id = ?`,
			actor.OrganizationID, projectID, document.ID)
		_ = s.Blobs.Delete(ctx, document.Blob)
		return Document{}, err
	}
	return document, nil
}

func (s Service) ListDocuments(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]Document, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, documentSelect+` WHERE organization_id = ? AND project_id = ?
		ORDER BY created_at DESC LIMIT ?`, actor.OrganizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Document{}
	for rows.Next() {
		value, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		value.ExtractedText = ""
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s Service) Search(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request SearchRequest) ([]SearchResult, error) {
	if request.Limit == 0 {
		request.Limit = 10
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.searchChunks(ctx, actor.OrganizationID, projectID, nil, request.Query, request.Limit)
}

func (s Service) GetDocument(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (Document, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return Document{}, err
	}
	return scanDocument(s.DB.QueryRowContext(ctx, documentSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`,
		actor.OrganizationID, projectID, id))
}

func (s Service) ExtractDocumentMedia(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) ([]ExtractedDocumentMedia, error) {
	document, err := s.GetDocument(ctx, actor, projectID, id)
	if err != nil {
		return nil, err
	}
	if document.MIMEType != "application/pdf" || s.Blobs == nil {
		return nil, ErrInvalidDocument
	}
	extractor, ok := s.DocumentParser.(DocumentMediaExtractor)
	if !ok {
		return nil, fmt.Errorf("document media extractor is unavailable")
	}
	stream, info, err := s.Blobs.Open(ctx, document.Blob)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return extractor.ExtractMedia(ctx, DocumentParseRequest{
		Filename: document.Filename, MIMEType: document.MIMEType, Size: info.SizeBytes, Source: stream,
	})
}

func (s Service) GetReference(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (Reference, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return Reference{}, err
	}
	document, err := scanDocument(s.DB.QueryRowContext(ctx, documentSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`,
		actor.OrganizationID, projectID, id))
	if err == nil {
		return Reference{
			ID: document.ID, Kind: "document", Title: document.Filename,
			Content: document.ExtractedText, ContentHash: document.TextSHA256,
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Reference{}, err
	}
	var artifact ResearchArtifact
	var sourceURL sql.NullString
	var citations []byte
	err = s.DB.QueryRowContext(ctx, `SELECT id, organization_id, project_id, research_run_id,
		source_type, title, source_url, content, citations, content_hash, created_at
		FROM platform_research_artifacts
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		actor.OrganizationID, projectID, id).Scan(
		&artifact.ID, &artifact.OrganizationID, &artifact.ProjectID, &artifact.ResearchRunID,
		&artifact.SourceType, &artifact.Title, &sourceURL, &artifact.Content, &citations,
		&artifact.ContentHash, &artifact.CreatedAt,
	)
	if err != nil {
		return Reference{}, err
	}
	artifact.SourceURL = sourceURL.String
	if err := json.Unmarshal(citations, &artifact.Citations); err != nil {
		return Reference{}, err
	}
	return Reference{
		ID: artifact.ID, Kind: "research_artifact", Title: artifact.Title,
		Content: artifact.Content, ContentHash: artifact.ContentHash,
		Citations: append([]string(nil), artifact.Citations...),
	}, nil
}

func (s Service) RunResearch(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request ResearchRequest) (ResearchRun, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return ResearchRun{}, err
	}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "web"
	}
	if !validResearchCategory(request.Category, true) {
		return ResearchRun{}, ErrInvalidResearchRequest
	}
	request.Category = normalizedResearchCategory(request.Category)
	request.Query = strings.TrimSpace(request.Query)
	if !request.Confirmed {
		return ResearchRun{}, ErrExternalConfirmationRequired
	}
	var err error
	request.Purpose, request.SourceRef, err = validateResearchContext(request.Purpose, request.SourceRef)
	if err != nil {
		return ResearchRun{}, err
	}
	request.DocumentIDs, request.DisclosedFields, err = validateResearchRequest(request)
	if err != nil {
		return ResearchRun{}, err
	}
	for _, id := range request.DocumentIDs {
		value, err := s.GetDocument(ctx, actor, projectID, id)
		if err != nil {
			return ResearchRun{}, err
		}
		if value.Status != "ready" || value.ChunkCount < 1 {
			return ResearchRun{}, ErrInvalidResearchRequest
		}
	}
	id, err := s.newID("researchrun")
	if err != nil {
		return ResearchRun{}, err
	}
	now := s.now()
	run := ResearchRun{
		ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		Mode: request.Mode, Category: request.Category, Purpose: request.Purpose, SourceRef: request.SourceRef, Query: request.Query,
		DocumentIDs:     append([]string(nil), request.DocumentIDs...),
		DisclosedFields: append([]string(nil), request.DisclosedFields...), Status: "running",
		ConfirmedBy: actor.Principal.ID, ConfirmedAt: now, Artifacts: []ResearchArtifact{},
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO platform_research_runs
		(id, organization_id, project_id, mode, category, purpose, source_type, source_id, query_text, document_ids, disclosed_fields,
		 disclosed_chunk_ids, status, confirmed_by, confirmed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.OrganizationID, run.ProjectID, run.Mode, run.Category, run.Purpose,
		nullableResourceType(run.SourceRef), nullableResourceID(run.SourceRef), run.Query, jsonBytes(run.DocumentIDs),
		jsonBytes(run.DisclosedFields), jsonBytes(run.DisclosedChunkIDs),
		run.Status, run.ConfirmedBy, run.ConfirmedAt, now, now); err != nil {
		return ResearchRun{}, err
	}
	if s.Runner == nil {
		run.Status, run.ErrorCode, run.ErrorMessage = "unavailable", "EXTERNAL_RUNNER_UNAVAILABLE", ErrExternalRunnerUnavailable.Error()
		run.UpdatedAt = s.now()
		_, _ = s.DB.ExecContext(ctx, `UPDATE platform_research_runs SET status = ?, error_code = ?,
			error_message = ?, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ?`,
			run.Status, run.ErrorCode, run.ErrorMessage, run.UpdatedAt, run.OrganizationID, run.ProjectID, run.ID)
		return run, nil
	}
	if s.Scheduler != nil {
		if err := s.Scheduler.Schedule(ctx, run); err != nil {
			run.Status, run.ErrorCode, run.ErrorMessage = "failed", "RESEARCH_SCHEDULE_FAILED", "研究任务暂时无法进入执行队列"
			run.UpdatedAt = s.now()
			_, _ = s.DB.ExecContext(ctx, `UPDATE platform_research_runs SET status = ?, error_code = ?,
				error_message = ?, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ?`,
				run.Status, run.ErrorCode, run.ErrorMessage, run.UpdatedAt, actor.OrganizationID, projectID, run.ID)
		}
		return run, nil
	}
	documents, err := s.selectResearchChunks(
		ctx, run.OrganizationID, run.ProjectID, run.DocumentIDs, run.Query,
	)
	if err != nil {
		return ResearchRun{}, err
	}
	return s.executeResearch(ctx, run, documents)
}

// RunConversationWebSearch executes the query before the owning conversation
// agent generates its answer. Unlike external research, it deliberately skips
// the standalone research scheduler: the durable AgentTask is already the
// orchestration boundary and must not answer until this run is terminal.
//
// A completed run is reused on AgentTask retry so a model failure after search
// does not issue the same paid web request again. A running run is either a
// legacy scheduled search (wait for it) or an interrupted inline run (resume
// it); the source-level unique key prevents concurrent duplicate creation.
func (s Service) RunConversationWebSearch(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	messageID string,
	query string,
) (ResearchRun, error) {
	messageID = strings.TrimSpace(messageID)
	query = strings.TrimSpace(query)
	if messageID == "" || query == "" {
		return ResearchRun{}, ErrInvalidResearchRequest
	}
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return ResearchRun{}, err
	}
	existing, found, err := s.conversationWebSearchRun(ctx, actor.OrganizationID, projectID, messageID)
	if err != nil {
		return ResearchRun{}, err
	}
	if found {
		if strings.TrimSpace(existing.Query) != query {
			return ResearchRun{}, ErrInvalidResearchRequest
		}
		if existing.Status == "running" {
			scheduled, err := s.conversationWebSearchHasScheduledJob(ctx, existing)
			if err != nil {
				return ResearchRun{}, err
			}
			if !scheduled {
				documents, err := s.selectResearchChunks(
					ctx, existing.OrganizationID, existing.ProjectID, existing.DocumentIDs, existing.Query,
				)
				if err != nil {
					return ResearchRun{}, err
				}
				resumed, err := s.executeResearch(ctx, existing, documents)
				if err != nil {
					return ResearchRun{}, err
				}
				return s.waitForConversationWebSearch(ctx, resumed)
			}
		}
		return s.waitForConversationWebSearch(ctx, existing)
	}

	inline := s
	inline.Scheduler = nil
	run, err := inline.RunResearch(ctx, actor, projectID, ResearchRequest{
		Mode: "web", Category: "general", Purpose: "conversation_web_search",
		SourceRef: &contract.ResourceRef{Type: "strategy_message", ID: messageID},
		Query:     query, DocumentIDs: []string{}, DisclosedFields: []string{"query"}, Confirmed: true,
	})
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			existing, found, loadErr := s.conversationWebSearchRun(ctx, actor.OrganizationID, projectID, messageID)
			if loadErr != nil {
				return ResearchRun{}, loadErr
			}
			if found {
				return s.waitForConversationWebSearch(ctx, existing)
			}
		}
		return ResearchRun{}, err
	}
	if run.Status != "succeeded" || len(run.Artifacts) == 0 {
		return run, fmt.Errorf("conversation web search did not complete: %s", run.ErrorCode)
	}
	return run, nil
}

func (s Service) conversationWebSearchHasScheduledJob(ctx context.Context, run ResearchRun) (bool, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_jobs
		WHERE organization_id = ? AND idempotency_key = ?`,
		run.OrganizationID, "knowledge_research_"+run.ID).Scan(&count)
	return count > 0, err
}

func (s Service) conversationWebSearchRun(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	messageID string,
) (ResearchRun, bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM platform_research_runs
		WHERE organization_id = ? AND project_id = ? AND purpose = 'conversation_web_search'
		  AND source_type = 'strategy_message' AND source_id = ?
		ORDER BY created_at DESC LIMIT 1`, organizationID, projectID, messageID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ResearchRun{}, false, nil
	}
	if err != nil {
		return ResearchRun{}, false, err
	}
	run, err := s.getResearchRun(ctx, organizationID, projectID, id)
	return run, err == nil, err
}

func (s Service) waitForConversationWebSearch(ctx context.Context, run ResearchRun) (ResearchRun, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		switch run.Status {
		case "succeeded":
			if len(run.Artifacts) == 0 {
				return run, fmt.Errorf("conversation web search returned no artifact")
			}
			return run, nil
		case "failed", "unavailable":
			return run, fmt.Errorf("conversation web search did not complete: %s", run.ErrorCode)
		case "running":
		default:
			return run, fmt.Errorf("conversation web search has invalid status %q", run.Status)
		}
		select {
		case <-ctx.Done():
			return ResearchRun{}, ctx.Err()
		case <-ticker.C:
			var err error
			run, err = s.getResearchRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
			if err != nil {
				return ResearchRun{}, err
			}
		}
	}
}

func (s Service) executeResearch(ctx context.Context, run ResearchRun, documents []ExternalDocument) (ResearchRun, error) {
	run.DisclosedChunkIDs = make([]string, 0, len(documents))
	for _, document := range documents {
		run.DisclosedChunkIDs = append(run.DisclosedChunkIDs, document.ID)
	}
	run.UpdatedAt = s.now()
	if _, err := s.DB.ExecContext(ctx, `UPDATE platform_research_runs
		SET disclosed_chunk_ids = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND status = 'running'`,
		jsonBytes(run.DisclosedChunkIDs), run.UpdatedAt,
		run.OrganizationID, run.ProjectID, run.ID); err != nil {
		return ResearchRun{}, err
	}
	results, err := s.Runner.Run(ctx, ExternalResearchInput{
		OrganizationID: run.OrganizationID,
		ProjectID:      run.ProjectID,
		Mode:           run.Mode,
		Category:       run.Category,
		Purpose:        run.Purpose,
		Query:          run.Query,
		Documents:      documents,
	})
	if err != nil {
		run.Status, run.ErrorCode, run.ErrorMessage = "failed", "EXTERNAL_RESEARCH_FAILED", "外部研究调用失败"
		run.UpdatedAt = s.now()
		_, _ = s.DB.ExecContext(ctx, `UPDATE platform_research_runs SET status = ?, error_code = ?,
			error_message = ?, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ?`,
			run.Status, run.ErrorCode, run.ErrorMessage, run.UpdatedAt, run.OrganizationID, run.ProjectID, run.ID)
		return run, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ResearchRun{}, err
	}
	defer tx.Rollback()
	for _, result := range results {
		artifact, err := s.insertArtifact(ctx, tx, run, result)
		if err != nil {
			return ResearchRun{}, err
		}
		run.Artifacts = append(run.Artifacts, artifact)
		if run.ProviderCode == "" {
			run.ProviderCode = strings.TrimSpace(result.ProviderCode)
			run.ModelVersion = strings.TrimSpace(result.ModelVersion)
			run.ProviderResponseID = strings.TrimSpace(result.ProviderResponse)
			run.Usage = result.Usage
		}
	}
	run.Status = "succeeded"
	run.UpdatedAt = s.now()
	if _, err := tx.ExecContext(ctx, `UPDATE platform_research_runs SET status = ?,
		provider_code = ?, model_version = ?, provider_response_id = ?, usage_json = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		run.Status, nullable(run.ProviderCode), nullable(run.ModelVersion),
		nullable(run.ProviderResponseID), nullableJSONValue(run.Usage), run.UpdatedAt,
		run.OrganizationID, run.ProjectID, run.ID); err != nil {
		return ResearchRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return ResearchRun{}, err
	}
	return run, nil
}

func (s Service) GetResearchRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (ResearchRun, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return ResearchRun{}, err
	}
	return s.getResearchRun(ctx, actor.OrganizationID, projectID, id)
}

func (s Service) ListResearchRuns(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]ResearchRun, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, researchRunSelect+`
		WHERE organization_id = ? AND project_id = ?
		ORDER BY created_at DESC LIMIT ?`, actor.OrganizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []ResearchRun{}
	for rows.Next() {
		value, err := scanResearchRun(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index].Artifacts, err = s.listResearchArtifacts(ctx, values[index].OrganizationID, values[index].ProjectID, values[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

const researchRunSelect = `SELECT id, organization_id, project_id, mode, query_text,
	category, purpose, COALESCE(source_type, ''), COALESCE(source_id, ''), document_ids, disclosed_fields, disclosed_chunk_ids, status, confirmed_by, confirmed_at,
	COALESCE(error_code, ''), COALESCE(error_message, ''),
	COALESCE(provider_code, ''), COALESCE(model_version, ''),
	COALESCE(provider_response_id, ''), usage_json, created_at, updated_at
	FROM platform_research_runs`

type researchRunScanner interface {
	Scan(...any) error
}

func scanResearchRun(scanner researchRunScanner) (ResearchRun, error) {
	var value ResearchRun
	var documentIDs, disclosedFields, disclosedChunkIDs []byte
	var usage []byte
	var sourceType, sourceID string
	err := scanner.Scan(
		&value.ID, &value.OrganizationID, &value.ProjectID, &value.Mode, &value.Query,
		&value.Category, &value.Purpose, &sourceType, &sourceID, &documentIDs, &disclosedFields, &disclosedChunkIDs,
		&value.Status, &value.ConfirmedBy, &value.ConfirmedAt,
		&value.ErrorCode, &value.ErrorMessage, &value.ProviderCode, &value.ModelVersion,
		&value.ProviderResponseID, &usage, &value.CreatedAt, &value.UpdatedAt,
	)
	if err != nil {
		return ResearchRun{}, err
	}
	if sourceType != "" && sourceID != "" {
		value.SourceRef = &contract.ResourceRef{Type: sourceType, ID: sourceID}
	}
	if err := json.Unmarshal(documentIDs, &value.DocumentIDs); err != nil {
		return ResearchRun{}, err
	}
	if err := json.Unmarshal(disclosedFields, &value.DisclosedFields); err != nil {
		return ResearchRun{}, err
	}
	if err := json.Unmarshal(disclosedChunkIDs, &value.DisclosedChunkIDs); err != nil {
		return ResearchRun{}, err
	}
	if len(usage) > 0 {
		value.Usage = &ResearchUsage{}
		if err := json.Unmarshal(usage, value.Usage); err != nil {
			return ResearchRun{}, err
		}
	}
	value.Artifacts = []ResearchArtifact{}
	return value, nil
}

func (s Service) getResearchRun(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (ResearchRun, error) {
	value, err := scanResearchRun(s.DB.QueryRowContext(ctx, researchRunSelect+`
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		organizationID, projectID, strings.TrimSpace(id)))
	if err != nil {
		return ResearchRun{}, err
	}
	value.Artifacts, err = s.listResearchArtifacts(ctx, organizationID, projectID, value.ID)
	if err != nil {
		return ResearchRun{}, err
	}
	return value, nil
}

func (s Service) listResearchArtifacts(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string) ([]ResearchArtifact, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, organization_id, project_id, research_run_id,
		source_type, category, title, COALESCE(source_url, ''), content, citations, content_hash, created_at
		FROM platform_research_artifacts
		WHERE organization_id = ? AND project_id = ? AND research_run_id = ?
		ORDER BY created_at ASC`, organizationID, projectID, runID)
	if err != nil {
		return nil, err
	}
	values := []ResearchArtifact{}
	for rows.Next() {
		var value ResearchArtifact
		var citations []byte
		if err := rows.Scan(
			&value.ID, &value.OrganizationID, &value.ProjectID, &value.ResearchRunID,
			&value.SourceType, &value.Category, &value.Title, &value.SourceURL, &value.Content, &citations,
			&value.ContentHash, &value.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(citations, &value.Citations); err != nil {
			return nil, err
		}
		value.Sources = []ResearchSource{}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index].Sources, err = s.listArtifactSources(
			ctx, string(organizationID), string(projectID), values[index].ID,
		)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s Service) ListResearchArtifacts(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, category string, limit int) ([]ResearchArtifact, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := `SELECT id, organization_id, project_id, research_run_id, source_type, category,
		title, COALESCE(source_url, ''), content, citations, content_hash, created_at
		FROM platform_research_artifacts
		WHERE organization_id = ? AND project_id = ?`
	args := []any{actor.OrganizationID, projectID}
	if category = strings.ToLower(strings.TrimSpace(category)); category != "" && category != "all" {
		if !validResearchCategory(category, false) {
			return nil, ErrInvalidResearchRequest
		}
		query += ` AND category = ?`
		args = append(args, normalizedResearchCategory(category))
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	values := make([]ResearchArtifact, 0)
	for rows.Next() {
		value, err := scanResearchArtifact(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		values[index].Sources, err = s.listArtifactSources(
			ctx, string(actor.OrganizationID), string(projectID), values[index].ID,
		)
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s Service) GetResearchArtifact(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (ResearchArtifact, error) {
	if _, err := s.Projects.GetContext(ctx, actor, projectID); err != nil {
		return ResearchArtifact{}, err
	}
	value, err := scanResearchArtifact(s.DB.QueryRowContext(ctx, `SELECT id, organization_id, project_id,
		research_run_id, source_type, category, title, COALESCE(source_url, ''), content,
		citations, content_hash, created_at FROM platform_research_artifacts
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		actor.OrganizationID, projectID, strings.TrimSpace(id)))
	if err != nil {
		return ResearchArtifact{}, err
	}
	value.Sources, err = s.listArtifactSources(ctx, string(actor.OrganizationID), string(projectID), value.ID)
	return value, err
}

func scanResearchArtifact(scanner interface{ Scan(...any) error }) (ResearchArtifact, error) {
	var value ResearchArtifact
	var citations []byte
	if err := scanner.Scan(
		&value.ID, &value.OrganizationID, &value.ProjectID, &value.ResearchRunID,
		&value.SourceType, &value.Category, &value.Title, &value.SourceURL, &value.Content,
		&citations, &value.ContentHash, &value.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResearchArtifact{}, ErrNotFound
		}
		return ResearchArtifact{}, err
	}
	if err := json.Unmarshal(citations, &value.Citations); err != nil {
		return ResearchArtifact{}, err
	}
	value.Sources = []ResearchSource{}
	return value, nil
}

func validateResearchRequest(request ResearchRequest) ([]string, []string, error) {
	if request.Mode != "web" || request.Query == "" ||
		len(request.Query) > 2000 || len(request.DocumentIDs) > 20 {
		return nil, nil, ErrInvalidResearchRequest
	}
	documentIDs := make([]string, 0, len(request.DocumentIDs))
	seenDocuments := map[string]struct{}{}
	for _, value := range request.DocumentIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil, ErrInvalidResearchRequest
		}
		if _, exists := seenDocuments[value]; exists {
			continue
		}
		seenDocuments[value] = struct{}{}
		documentIDs = append(documentIDs, value)
	}
	disclosed := map[string]struct{}{}
	for _, value := range request.DisclosedFields {
		value = strings.TrimSpace(value)
		if value != "query" && value != "document_content" {
			return nil, nil, ErrInvalidResearchRequest
		}
		disclosed[value] = struct{}{}
	}
	if _, ok := disclosed["query"]; !ok {
		return nil, nil, ErrInvalidResearchRequest
	}
	_, disclosesDocuments := disclosed["document_content"]
	if disclosesDocuments != (len(documentIDs) > 0) {
		return nil, nil, ErrInvalidResearchRequest
	}
	fields := []string{"query"}
	if disclosesDocuments {
		fields = append(fields, "document_content")
	}
	return documentIDs, fields, nil
}

func validateResearchContext(purpose string, sourceRef *contract.ResourceRef) (string, *contract.ResourceRef, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "deep_research"
	}
	switch purpose {
	case "deep_research":
		if sourceRef != nil {
			return "", nil, ErrInvalidResearchRequest
		}
		return purpose, nil, nil
	case "conversation_web_search":
		if sourceRef == nil || sourceRef.Type != "strategy_message" || strings.TrimSpace(sourceRef.ID) == "" {
			return "", nil, ErrInvalidResearchRequest
		}
		return purpose, &contract.ResourceRef{Type: sourceRef.Type, ID: strings.TrimSpace(sourceRef.ID)}, nil
	default:
		return "", nil, ErrInvalidResearchRequest
	}
}

func nullableResourceType(ref *contract.ResourceRef) any {
	if ref == nil || strings.TrimSpace(ref.Type) == "" {
		return nil
	}
	return strings.TrimSpace(ref.Type)
}

func nullableResourceID(ref *contract.ResourceRef) any {
	if ref == nil || strings.TrimSpace(ref.ID) == "" {
		return nil
	}
	return strings.TrimSpace(ref.ID)
}

func (s Service) insertArtifact(ctx context.Context, tx *sql.Tx, run ResearchRun, result ExternalResearchResult) (ResearchArtifact, error) {
	if strings.TrimSpace(result.Title) == "" || strings.TrimSpace(result.Content) == "" {
		return ResearchArtifact{}, fmt.Errorf("external research returned an incomplete artifact")
	}
	id, err := s.newID("researchartifact")
	if err != nil {
		return ResearchArtifact{}, err
	}
	hash, err := contract.CanonicalJSONHash(result)
	if err != nil {
		return ResearchArtifact{}, err
	}
	value := ResearchArtifact{
		ID: id, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID,
		ResearchRunID: run.ID, SourceType: run.Mode, Category: run.Category,
		Title:     strings.TrimSpace(result.Title),
		SourceURL: strings.TrimSpace(result.SourceURL), Content: strings.TrimSpace(result.Content),
		Citations: append([]string(nil), result.Citations...), ContentHash: hash, CreatedAt: s.now(),
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_research_artifacts
		(id, organization_id, project_id, research_run_id, source_type, category, title, source_url,
		 content, citations, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.ResearchRunID, value.SourceType, value.Category,
		value.Title, nullable(value.SourceURL), value.Content, jsonBytes(value.Citations),
		value.ContentHash, value.CreatedAt)
	if err != nil {
		return ResearchArtifact{}, err
	}
	value.Sources = make([]ResearchSource, 0, len(result.Sources))
	for _, source := range result.Sources {
		inserted, insertErr := s.insertResearchSource(ctx, tx, run, value.ID, source)
		if insertErr != nil {
			return ResearchArtifact{}, insertErr
		}
		value.Sources = append(value.Sources, inserted)
	}
	return value, nil
}

func normalizedResearchCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "audience", "competitor", "industry":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "general"
	}
}

func validResearchCategory(value string, allowEmpty bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return allowEmpty
	case "general", "audience", "competitor", "industry":
		return true
	default:
		return false
	}
}

func extractDocument(extension string, content []byte) (string, string, error) {
	switch extension {
	case ".md":
		content = bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
		if !utf8.Valid(content) {
			return "", "", ErrInvalidDocument
		}
		return strings.TrimSpace(string(content)), "text/markdown", nil
	case ".docx":
		reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			return "", "", ErrInvalidDocument
		}
		for _, file := range reader.File {
			if file.Name != "word/document.xml" || file.UncompressedSize64 > uint64(maxExtractedBytes) {
				continue
			}
			stream, err := file.Open()
			if err != nil {
				return "", "", ErrInvalidDocument
			}
			text, err := extractWordXML(io.LimitReader(stream, maxExtractedBytes+1))
			_ = stream.Close()
			if err != nil || text == "" {
				return "", "", ErrInvalidDocument
			}
			return text, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
		}
	}
	return "", "", ErrInvalidDocument
}

func extractWordXML(source io.Reader) (string, error) {
	decoder := xml.NewDecoder(source)
	var builder strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "t" {
				var text string
				if err := decoder.DecodeElement(&text, &value); err != nil {
					return "", err
				}
				builder.WriteString(text)
			} else if value.Name.Local == "tab" {
				builder.WriteByte('\t')
			} else if value.Name.Local == "br" {
				builder.WriteByte('\n')
			}
		case xml.EndElement:
			if value.Name.Local == "p" {
				builder.WriteByte('\n')
			}
		}
		if int64(builder.Len()) > maxExtractedBytes {
			return "", ErrInvalidDocument
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

func allowedMIME(extension, value string) bool {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	switch extension {
	case ".md":
		return value == "text/markdown" || value == "text/plain" || value == "application/octet-stream"
	case ".docx":
		return value == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
			value == "application/octet-stream"
	case ".pdf":
		return value == "application/pdf" || value == "application/octet-stream"
	default:
		return false
	}
}

func defaultDocumentMIME(extension string) string {
	switch extension {
	case ".md":
		return "text/markdown"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

const documentSelect = `SELECT id, organization_id, project_id, title, COALESCE(source_uri, ''),
	source_type, chunk_count, filename, mime_type, size_bytes,
	content_sha256, text_sha256, extracted_text, status,
	COALESCE(parser_code, ''), COALESCE(parser_version, ''),
	COALESCE(parse_error_code, ''), COALESCE(parse_error_message, ''),
	COALESCE(parse_metadata, JSON_OBJECT()), parsed_at,
	created_by, created_at, updated_at,
	object_provider, object_bucket, object_key, object_version_id, object_etag
	FROM platform_knowledge_documents`

type scanner interface {
	Scan(...any) error
}

func scanDocument(row scanner) (Document, error) {
	var value Document
	var versionID, etag sql.NullString
	var parsedAt sql.NullTime
	err := row.Scan(
		&value.ID, &value.OrganizationID, &value.ProjectID, &value.Title, &value.SourceURI,
		&value.SourceType, &value.ChunkCount, &value.Filename, &value.MIMEType,
		&value.SizeBytes, &value.ContentSHA256, &value.TextSHA256, &value.ExtractedText,
		&value.Status, &value.ParserCode, &value.ParserVersion,
		&value.ParseErrorCode, &value.ParseErrorMessage, &value.ParseMetadata, &parsedAt,
		&value.CreatedBy, &value.CreatedAt, &value.UpdatedAt,
		&value.Blob.Provider, &value.Blob.Bucket, &value.Blob.Key, &versionID, &etag,
	)
	value.Blob.VersionID, value.Blob.ETag = versionID.String, etag.String
	if parsedAt.Valid {
		value.ParsedAt = &parsedAt.Time
	}
	return value, err
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) (string, error) {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return ids.New(prefix)
}

func jsonBytes(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func nullableJSONValue(value any) any {
	if value == nil {
		return nil
	}
	return jsonBytes(value)
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

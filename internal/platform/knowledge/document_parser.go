package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shikanon/cookies/internal/platform/assets"
)

type DocumentParseRequest struct {
	Filename string
	MIMEType string
	Size     int64
	Source   io.Reader
}

type ParsedDocument struct {
	Text          string
	MIMEType      string
	ParserCode    string
	ParserVersion string
	Metadata      json.RawMessage
}

type ExtractedDocumentMedia struct {
	Filename   string `json:"filename"`
	MIMEType   string `json:"mime_type"`
	PageNumber int    `json:"page_number"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	PageText   string `json:"page_text,omitempty"`
	Content    []byte `json:"content"`
}

type DocumentParser interface {
	Parse(context.Context, DocumentParseRequest) (ParsedDocument, error)
}

type DocumentMediaExtractor interface {
	ExtractMedia(context.Context, DocumentParseRequest) ([]ExtractedDocumentMedia, error)
}

type TikaParser struct {
	BaseURL        string
	Version        string
	Timeout        time.Duration
	MaxOutputBytes int64
	HTTPClient     *http.Client
}

const (
	maxTikaMediaResources    = 200
	maxTikaMediaArchiveBytes = 20 * 1024 * 1024
)

func (p TikaParser) ExtractMedia(ctx context.Context, input DocumentParseRequest) ([]ExtractedDocumentMedia, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" || input.Source == nil || input.Size < 1 || input.MIMEType != "application/pdf" {
		return nil, fmt.Errorf("Tika media extraction request is invalid")
	}
	source, err := io.ReadAll(io.LimitReader(input.Source, MaxDocumentBytes+1))
	if err != nil || int64(len(source)) > MaxDocumentBytes {
		return nil, fmt.Errorf("PDF source exceeded the media extraction limit")
	}
	metadataBody, err := p.requestInlineMedia(ctx, baseURL+"/rmeta/html", input, source, "application/json", 2*1024*1024)
	if err != nil {
		return nil, err
	}
	var records []map[string]any
	if err := json.Unmarshal(metadataBody, &records); err != nil {
		return nil, fmt.Errorf("decode Tika media metadata: %w", err)
	}
	pageTexts := []string{}
	if len(records) > 0 {
		pageTexts = tikaPageTexts(metadataString(records[0], "X-TIKA:content"))
	}
	archiveBody, err := p.requestInlineMedia(ctx, baseURL+"/unpack/all", input, source, "application/zip", maxTikaMediaArchiveBytes)
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(archiveBody), int64(len(archiveBody)))
	if err != nil {
		return nil, fmt.Errorf("decode Tika media archive: %w", err)
	}
	entries := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		entries[path.Base(file.Name)] = file
	}
	result := make([]ExtractedDocumentMedia, 0, min(len(records), maxTikaMediaResources))
	for _, record := range records {
		filename := path.Base(metadataString(record, "resourceName"))
		mimeType := metadataString(record, "Content-Type")
		if filename == "." || filename == "" || (mimeType != "image/png" && mimeType != "image/jpeg") {
			continue
		}
		file := entries[filename]
		if file == nil || file.UncompressedSize64 == 0 || file.UncompressedSize64 > uint64(assets.MaxImageBytes) {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			continue
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, assets.MaxImageBytes+1))
		_ = stream.Close()
		if readErr != nil || int64(len(content)) > assets.MaxImageBytes {
			continue
		}
		digest := sha256.Sum256(content)
		pageNumber := metadataInt(record, "tika_pg:page_number")
		pageText := ""
		if pageNumber > 0 && pageNumber <= len(pageTexts) {
			pageText = pageTexts[pageNumber-1]
		}
		result = append(result, ExtractedDocumentMedia{
			Filename: filename, MIMEType: mimeType,
			PageNumber: pageNumber,
			Width:      firstMetadataInt(record, "width", "Image Width", "tiff:ImageWidth"),
			Height:     firstMetadataInt(record, "height", "Image Height", "tiff:ImageLength"),
			SizeBytes:  int64(len(content)), SHA256: hex.EncodeToString(digest[:]), PageText: pageText, Content: content,
		})
		if len(result) == maxTikaMediaResources {
			break
		}
	}
	return result, nil
}

var tikaPagePattern = regexp.MustCompile(`(?is)<div\s+class=["']page["']\s*>(.*?)</div>`)
var tikaTagPattern = regexp.MustCompile(`(?is)<[^>]+>`)
var tikaSpacePattern = regexp.MustCompile(`\s+`)

func tikaPageTexts(value string) []string {
	matches := tikaPagePattern.FindAllStringSubmatch(value, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		plain := html.UnescapeString(tikaTagPattern.ReplaceAllString(match[1], " "))
		result = append(result, strings.TrimSpace(tikaSpacePattern.ReplaceAllString(plain, " ")))
	}
	return result
}

func (p TikaParser) requestInlineMedia(ctx context.Context, url string, input DocumentParseRequest, source []byte, accept string, limit int64) ([]byte, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPut, url, bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("build Tika media request: %w", err)
	}
	request.Header.Set("Content-Type", input.MIMEType)
	request.Header.Set("Accept", accept)
	request.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": input.Filename}))
	request.Header.Set("X-Tika-PDFextractInlineImages", "true")
	request.Header.Set("X-Tika-PDFextractUniqueInlineImagesOnly", "true")
	request.Header.Set("maxEmbeddedResources", strconv.Itoa(maxTikaMediaResources))
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Tika media extraction request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fmt.Errorf("Tika media extraction response exceeded the safety limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Tika media extraction returned HTTP %d", response.StatusCode)
	}
	return body, nil
}

func metadataString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func metadataInt(record map[string]any, key string) int {
	value := metadataString(record, key)
	value = strings.TrimSpace(strings.TrimSuffix(value, " pixels"))
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func firstMetadataInt(record map[string]any, keys ...string) int {
	for _, key := range keys {
		if value := metadataInt(record, key); value > 0 {
			return value
		}
	}
	return 0
}

func (p TikaParser) Parse(ctx context.Context, input DocumentParseRequest) (ParsedDocument, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" || input.Source == nil || input.Size < 1 || strings.TrimSpace(input.MIMEType) == "" {
		return ParsedDocument{}, fmt.Errorf("Tika parse request is invalid")
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	limit := p.MaxOutputBytes
	if limit < 1 {
		limit = maxExtractedBytes
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPut, baseURL+"/rmeta/text", input.Source)
	if err != nil {
		return ParsedDocument{}, fmt.Errorf("build Tika request: %w", err)
	}
	request.Header.Set("Content-Type", input.MIMEType)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": input.Filename,
	}))
	request.Header.Set("maxEmbeddedResources", "0")
	request.Header.Set("writeLimit", fmt.Sprint(limit))
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		if requestCtx.Err() == context.DeadlineExceeded {
			return ParsedDocument{}, fmt.Errorf("Tika parsing timed out")
		}
		return ParsedDocument{}, fmt.Errorf("Tika parsing request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return ParsedDocument{}, fmt.Errorf("Tika parsing response exceeded the safety limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ParsedDocument{}, fmt.Errorf("Tika parsing returned HTTP %d", response.StatusCode)
	}
	var records []map[string]any
	if err := json.Unmarshal(body, &records); err != nil || len(records) == 0 {
		return ParsedDocument{}, fmt.Errorf("Tika parsing response is invalid")
	}
	contents := make([]string, 0, len(records))
	for _, record := range records {
		if value, ok := record["X-TIKA:content"].(string); ok && strings.TrimSpace(value) != "" {
			contents = append(contents, strings.TrimSpace(value))
		}
		delete(record, "X-TIKA:content")
	}
	text := strings.TrimSpace(strings.Join(contents, "\n\n"))
	if text == "" || !utf8.ValidString(text) {
		return ParsedDocument{}, fmt.Errorf("Tika parsing returned no valid text")
	}
	metadata, _ := json.Marshal(records)
	version := strings.TrimSpace(p.Version)
	if version == "" {
		version = "unknown"
	}
	return ParsedDocument{
		Text: text, MIMEType: input.MIMEType,
		ParserCode: "tika", ParserVersion: version, Metadata: metadata,
	}, nil
}

func chunksForParsedDocument(document Document, parsed ParsedDocument) []Chunk {
	const targetRunes = 800
	lines := strings.Split(strings.ReplaceAll(parsed.Text, "\r\n", "\n"), "\n")
	chunks := make([]Chunk, 0)
	var buffer bytes.Buffer
	startLine := 1
	flush := func(endLine int) {
		text := strings.TrimSpace(buffer.String())
		buffer.Reset()
		if text == "" {
			startLine = endLine + 1
			return
		}
		sum := sha256.Sum256([]byte(text))
		textHash := hex.EncodeToString(sum[:])
		ordinal := len(chunks)
		idInput := fmt.Sprintf("%s|%s|%s|%s|%d|%s",
			document.ID, document.ContentSHA256, parsed.ParserCode, parsed.ParserVersion, ordinal, textHash)
		idSum := sha256.Sum256([]byte(idInput))
		chunks = append(chunks, Chunk{
			ID:         "knowledgechunk_" + hex.EncodeToString(idSum[:24]),
			DocumentID: document.ID, OrganizationID: document.OrganizationID, ProjectID: document.ProjectID,
			Index: ordinal, Kind: "text", Text: text, SourceURI: document.SourceURI,
			Section: "正文", StartLine: startLine, EndLine: endLine,
			TextSHA256: textHash, Locator: map[string]any{
				"section": "正文", "start_line": startLine, "end_line": endLine,
			},
			ParserCode: parsed.ParserCode, ParserVersion: parsed.ParserVersion,
			CreatedAt: document.UpdatedAt,
		})
		startLine = endLine + 1
	}
	for index, line := range lines {
		lineNumber := index + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if buffer.Len() > 0 && utf8.RuneCount(buffer.Bytes()) >= targetRunes/2 {
				flush(lineNumber)
			}
			continue
		}
		lineRunes := []rune(trimmed)
		for len(lineRunes) > 0 {
			take := min(len(lineRunes), targetRunes)
			segment := string(lineRunes[:take])
			lineRunes = lineRunes[take:]
			if buffer.Len() > 0 &&
				utf8.RuneCount(buffer.Bytes())+1+utf8.RuneCountInString(segment) > targetRunes {
				flush(max(lineNumber-1, startLine))
				startLine = lineNumber
			}
			if buffer.Len() > 0 {
				buffer.WriteByte('\n')
			}
			buffer.WriteString(segment)
			if len(lineRunes) > 0 || utf8.RuneCount(buffer.Bytes()) >= targetRunes {
				flush(lineNumber)
				startLine = lineNumber
			}
		}
	}
	if buffer.Len() > 0 {
		flush(len(lines))
	}
	return chunks
}

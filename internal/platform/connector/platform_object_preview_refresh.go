package connector

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type pictureURISigner interface {
	SignPictureURIs(context.Context, []string) (map[string]any, error)
}

type platformObjectPreviewStore interface {
	PlatformObjectPreviewReader
	updatePlatformObjectPreview(context.Context, PlatformObjectPreviewQuery, string, string, *time.Time, time.Time) error
}

func (s Synchronizer) RefreshPlatformObjectPreview(ctx context.Context, query PlatformObjectPreviewQuery) (PlatformObjectPreview, error) {
	store, ok := s.Writer.(platformObjectPreviewStore)
	if !ok || s.Readers == nil {
		return PlatformObjectPreview{}, ErrInvalidFact
	}
	current, err := store.GetPlatformObjectPreview(ctx, query)
	if err != nil || (current.Kind != "image" && current.Kind != "video_poster") {
		return PlatformObjectPreview{}, ErrInvalidFact
	}
	uri, err := stablePictureURI(current.URL)
	if err != nil {
		return PlatformObjectPreview{}, fmt.Errorf("derive stable picture URI: %w", err)
	}
	reader, closeReader, err := s.Readers.Open(ctx, SyncRequest{OrganizationID: query.OrganizationID, ProjectID: query.ProjectID, AccountRef: query.AccountID})
	if err != nil {
		return PlatformObjectPreview{}, fmt.Errorf("open picture signer: %w", err)
	}
	if closeReader != nil {
		defer closeReader()
	}
	signer, ok := reader.(pictureURISigner)
	if !ok {
		return PlatformObjectPreview{}, fmt.Errorf("picture signer is unavailable: %w", ErrInvalidFact)
	}
	payload, err := signer.SignPictureURIs(ctx, []string{uri})
	if err != nil {
		return PlatformObjectPreview{}, fmt.Errorf("sign picture URI: %w", err)
	}
	signedURL := signedPictureURL(payload, uri)
	signedURL, expiresAt := platformPreview(signedURL)
	if signedURL == "" || expiresAt == nil || !time.Now().Before(*expiresAt) {
		return PlatformObjectPreview{}, fmt.Errorf("%w: picture signer returned no usable URL (%s)", ErrInvalidFact, signedPicturePayloadShape(payload))
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	if err := store.updatePlatformObjectPreview(ctx, query, signedURL, current.Kind, expiresAt, now); err != nil {
		return PlatformObjectPreview{}, err
	}
	return store.GetPlatformObjectPreview(ctx, query)
}

func stablePictureURI(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse preview URL: %w", ErrInvalidFact)
	}
	if parsed.Scheme != "https" || !previewMediaHostAllowed(parsed.Hostname()) {
		return "", fmt.Errorf("unsupported preview origin scheme=%q host=%q: %w", parsed.Scheme, parsed.Hostname(), ErrInvalidFact)
	}
	value := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if index := strings.Index(value, "~"); index >= 0 {
		value = value[:index]
	}
	value, err = url.PathUnescape(value)
	value = strings.TrimPrefix(value, "obj/")
	if err != nil || value == "" || len(value) > 2048 || strings.Contains(value, "..") {
		return "", ErrInvalidFact
	}
	return value, nil
}

func signedPictureURL(payload map[string]any, uri string) string {
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = payload
	}
	list, _ := data["list"].(map[string]any)
	item, _ := list[uri].(map[string]any)
	return firstString(item, "main_url")
}

func signedPicturePayloadShape(payload map[string]any) string {
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return fmt.Sprintf("data=%T", payload["data"])
	}
	return fmt.Sprintf("data.list=%T", data["list"])
}

package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PlatformObjectKind string

const (
	PlatformObjectImageMaterial      PlatformObjectKind = "image_material"
	PlatformObjectProductImage       PlatformObjectKind = "product_image"
	PlatformObjectVideoMaterial      PlatformObjectKind = "video_material"
	PlatformObjectAwemePhotoMaterial PlatformObjectKind = "aweme_photo_material"
	PlatformObjectMarketingProduct   PlatformObjectKind = "marketing_product"
	PlatformObjectOrangeLandingPage  PlatformObjectKind = "orange_landing_page"
	PlatformObjectOptimizationTarget PlatformObjectKind = "optimization_target"
	PlatformObjectConversionAsset    PlatformObjectKind = "conversion_event_asset"
	PlatformObjectIndustryCategory   PlatformObjectKind = "industry_category"
	PlatformObjectBrand              PlatformObjectKind = "brand"
	PlatformObjectAuthorizedIdentity PlatformObjectKind = "authorized_identity"
)

func (k PlatformObjectKind) Valid() bool {
	switch k {
	case PlatformObjectImageMaterial, PlatformObjectProductImage, PlatformObjectVideoMaterial, PlatformObjectAwemePhotoMaterial,
		PlatformObjectMarketingProduct, PlatformObjectOrangeLandingPage, PlatformObjectOptimizationTarget,
		PlatformObjectConversionAsset, PlatformObjectIndustryCategory, PlatformObjectBrand, PlatformObjectAuthorizedIdentity:
		return true
	default:
		return false
	}
}

type PlatformObjectCandidate struct {
	Kind             PlatformObjectKind
	PlatformObjectID string
	DisplayName      string
	Metadata         map[string]any
	PreviewURL       string
	PreviewKind      string
	PreviewExpiresAt *time.Time
}

type PlatformObject struct {
	ID               string                     `json:"id"`
	OrganizationID   string                     `json:"organization_id"`
	AccountID        string                     `json:"account_id"`
	Kind             PlatformObjectKind         `json:"object_kind"`
	PlatformObjectID string                     `json:"platform_object_id"`
	DisplayName      string                     `json:"display_name"`
	Status           string                     `json:"status"`
	Metadata         map[string]any             `json:"metadata"`
	ObservedAt       time.Time                  `json:"observed_at"`
	Version          int64                      `json:"version"`
	ProjectGranted   bool                       `json:"project_granted"`
	PreviewAvailable bool                       `json:"preview_available"`
	PreviewKind      string                     `json:"preview_kind,omitempty"`
	PreviewExpiresAt *time.Time                 `json:"preview_expires_at,omitempty"`
	PreviewURL       string                     `json:"preview_url,omitempty"`
	Performance      *PlatformObjectPerformance `json:"performance,omitempty"`
}

type PlatformObjectPerformance struct {
	Available   bool       `json:"available"`
	SpendMinor  int64      `json:"spend_minor"`
	Impressions int64      `json:"impressions"`
	Clicks      int64      `json:"clicks"`
	Conversions int64      `json:"conversions"`
	CTR         float64    `json:"ctr"`
	DataThrough *time.Time `json:"data_through,omitempty"`
}

type PlatformObjectPreviewQuery struct {
	OrganizationID string
	ProjectID      string
	AccountID      string
	ObjectID       string
}

type PlatformObjectPreview struct {
	URL        string
	Kind       string
	ObjectKind PlatformObjectKind
	ExpiresAt  *time.Time
}

type PlatformObjectPreviewReader interface {
	GetPlatformObjectPreview(context.Context, PlatformObjectPreviewQuery) (PlatformObjectPreview, error)
}

type PlatformObjectPreviewContent struct {
	ContentType string
	Data        []byte
}

type PlatformObjectPreviewContentReader interface {
	ReadPlatformObjectPreview(context.Context, PlatformObjectPreviewQuery) (PlatformObjectPreviewContent, error)
}

type PlatformObjectPreviewRefresher interface {
	RefreshPlatformObjectPreview(context.Context, PlatformObjectPreviewQuery) (PlatformObjectPreview, error)
}

type PlatformObjectSyncStats struct {
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Unchanged   int `json:"unchanged"`
	Unavailable int `json:"unavailable"`
}

type PlatformObjectQuery struct {
	OrganizationID string
	ProjectID      string
	AccountID      string
	Kind           PlatformObjectKind
	Status         string
	Search         string
	Cursor         string
	Limit          int
	SortBy         string
	SortOrder      string
	Offset         int
}

func validPlatformObjectSort(sortBy, sortOrder string) bool {
	if sortBy != "" && sortBy != "created_at" && sortBy != "ctr" && sortBy != "conversions" {
		return false
	}
	return sortOrder == "" || sortOrder == "asc" || sortOrder == "desc"
}

type PlatformObjectCatalog interface {
	ReconcilePlatformObjects(context.Context, string, string, string, string, PlatformObjectKind, time.Time, []PlatformObjectCandidate) (PlatformObjectSyncStats, error)
	ListPlatformObjects(context.Context, PlatformObjectQuery) ([]PlatformObject, error)
}

func PlatformObjectID(organizationID, accountID string, kind PlatformObjectKind, platformID string) string {
	return "oeobj_" + canonicalHash([]string{organizationID, accountID, string(kind), platformID})
}

func (r MySQLRepository) ReconcilePlatformObjects(ctx context.Context, organizationID, projectID, accountID, syncID string, kind PlatformObjectKind, observedAt time.Time, candidates []PlatformObjectCandidate) (PlatformObjectSyncStats, error) {
	var stats PlatformObjectSyncStats
	if organizationID == "" || projectID == "" || accountID == "" || syncID == "" || !kind.Valid() || observedAt.IsZero() {
		return stats, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return stats, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate.PlatformObjectID = strings.TrimSpace(candidate.PlatformObjectID)
		candidate.DisplayName = strings.TrimSpace(candidate.DisplayName)
		if candidate.Kind != kind || !validPlatformObjectID(kind, candidate.PlatformObjectID) || len(candidate.DisplayName) > 512 {
			return stats, ErrInvalidFact
		}
		objectID := PlatformObjectID(organizationID, accountID, kind, candidate.PlatformObjectID)
		if _, duplicate := seen[objectID]; duplicate {
			continue
		}
		seen[objectID] = struct{}{}
		metadata, err := json.Marshal(candidate.Metadata)
		if err != nil || len(metadata) > 32<<10 {
			return stats, ErrInvalidFact
		}
		fingerprint := canonicalHash([]string{string(kind), candidate.PlatformObjectID, candidate.DisplayName, string(metadata)})
		var previewCiphertext []byte
		var previewKeyVersion string
		if candidate.PreviewURL != "" {
			if r.Cipher == nil || !validPreviewKind(candidate.PreviewKind) {
				return stats, ErrInvalidFact
			}
			previewCiphertext, previewKeyVersion, err = r.Cipher.Encrypt([]byte(candidate.PreviewURL))
			if err != nil {
				return stats, fmt.Errorf("encrypt platform object preview: %w", err)
			}
		}
		var currentFingerprint, currentStatus string
		var currentVersion int64
		err = tx.QueryRowContext(ctx, `SELECT source_fingerprint,status,version FROM connector_platform_objects WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, objectID).Scan(&currentFingerprint, &currentStatus, &currentVersion)
		switch {
		case err == sql.ErrNoRows:
			_, err = tx.ExecContext(ctx, `INSERT INTO connector_platform_objects (id,organization_id,account_id,object_kind,platform_object_id,display_name,preview_kind,preview_url_ciphertext,preview_key_version,preview_expires_at,preview_observed_at,status,metadata_json,source_fingerprint,last_sync_id,observed_at,version,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,'active',?,?,?,?,1,?,?)`, objectID, organizationID, accountID, kind, candidate.PlatformObjectID, candidate.DisplayName, candidate.PreviewKind, nullableBytes(previewCiphertext), nullable(previewKeyVersion), candidate.PreviewExpiresAt, nullableTime(candidate.PreviewURL, observedAt), metadata, fingerprint, syncID, observedAt, observedAt, observedAt)
			stats.Created++
		case err != nil:
			return stats, err
		case currentFingerprint != fingerprint || currentStatus != "active":
			_, err = tx.ExecContext(ctx, `UPDATE connector_platform_objects SET display_name=?,preview_kind=IF(?='',preview_kind,?),preview_url_ciphertext=IF(? IS NULL,preview_url_ciphertext,?),preview_key_version=IF(?='',preview_key_version,?),preview_expires_at=IF(? IS NULL,preview_expires_at,?),preview_observed_at=IF(? IS NULL,preview_observed_at,?),status='active',metadata_json=?,source_fingerprint=?,last_sync_id=?,observed_at=?,version=?,updated_at=? WHERE organization_id=? AND id=?`, candidate.DisplayName, candidate.PreviewKind, candidate.PreviewKind, nullableBytes(previewCiphertext), nullableBytes(previewCiphertext), previewKeyVersion, previewKeyVersion, nullableBytes(previewCiphertext), candidate.PreviewExpiresAt, nullableTime(candidate.PreviewURL, observedAt), nullableTime(candidate.PreviewURL, observedAt), metadata, fingerprint, syncID, observedAt, currentVersion+1, observedAt, organizationID, objectID)
			stats.Updated++
		default:
			_, err = tx.ExecContext(ctx, `UPDATE connector_platform_objects SET preview_kind=IF(?='',preview_kind,?),preview_url_ciphertext=IF(? IS NULL,preview_url_ciphertext,?),preview_key_version=IF(?='',preview_key_version,?),preview_expires_at=IF(? IS NULL,preview_expires_at,?),preview_observed_at=IF(? IS NULL,preview_observed_at,?),last_sync_id=?,observed_at=?,updated_at=? WHERE organization_id=? AND id=?`, candidate.PreviewKind, candidate.PreviewKind, nullableBytes(previewCiphertext), nullableBytes(previewCiphertext), previewKeyVersion, previewKeyVersion, nullableBytes(previewCiphertext), candidate.PreviewExpiresAt, nullableTime(candidate.PreviewURL, observedAt), nullableTime(candidate.PreviewURL, observedAt), syncID, observedAt, observedAt, organizationID, objectID)
			stats.Unchanged++
		}
		if err != nil {
			return stats, fmt.Errorf("upsert platform object: %w", err)
		}
		grantID := "oegrant_" + canonicalHash([]string{organizationID, projectID, objectID})
		_, err = tx.ExecContext(ctx, `INSERT INTO connector_platform_object_project_grants (id,organization_id,project_id,platform_object_id,status,granted_at,updated_at) VALUES (?,?,?,?,'active',?,?) ON DUPLICATE KEY UPDATE status='active',updated_at=VALUES(updated_at)`, grantID, organizationID, projectID, objectID, observedAt, observedAt)
		if err != nil {
			return stats, fmt.Errorf("grant platform object: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE connector_platform_objects SET status='unavailable',version=version+1,updated_at=? WHERE organization_id=? AND account_id=? AND object_kind=? AND status='active' AND last_sync_id<>?`, observedAt, organizationID, accountID, kind, syncID)
	if err != nil {
		return stats, fmt.Errorf("mark unavailable platform objects: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return stats, err
	}
	stats.Unavailable = int(count)
	if err = tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

func (r MySQLRepository) ListPlatformObjects(ctx context.Context, query PlatformObjectQuery) ([]PlatformObject, error) {
	if query.OrganizationID == "" || query.ProjectID == "" || query.AccountID == "" || query.Offset < 0 || !validPlatformObjectSort(query.SortBy, query.SortOrder) || (query.Kind != "" && !query.Kind.Valid()) || (query.Status != "" && query.Status != "active" && query.Status != "unavailable") {
		return nil, ErrInvalidFact
	}
	limit := query.Limit
	if limit < 1 || limit > 200 {
		limit = 100
	}
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	statement := `SELECT o.id,o.organization_id,o.account_id,o.object_kind,o.platform_object_id,o.display_name,o.status,o.metadata_json,o.observed_at,o.version,o.preview_url_ciphertext IS NOT NULL,o.preview_kind,o.preview_expires_at,COALESCE(m.spend_minor,0),COALESCE(m.impressions,0),COALESCE(m.clicks,0),COALESCE(m.conversions,0),m.data_through FROM connector_platform_objects o JOIN connector_platform_object_project_grants g ON g.organization_id=o.organization_id AND g.platform_object_id=o.id AND g.project_id=? AND g.status='active' LEFT JOIN (SELECT organization_id,material_ref,SUM(COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metrics_json,'$.spend')) AS SIGNED),0)) spend_minor,SUM(COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metrics_json,'$.impressions')) AS SIGNED),0)) impressions,SUM(COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metrics_json,'$.clicks')) AS SIGNED),0)) clicks,SUM(COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metrics_json,'$.conversions')) AS SIGNED),0)) conversions,MAX(window_end) data_through FROM connector_material_metric_windows WHERE organization_id=? AND source_ref=? GROUP BY organization_id,material_ref) m ON m.organization_id=o.organization_id AND m.material_ref=CONCAT('ref_',SHA2(JSON_QUOTE(o.platform_object_id),256)) WHERE o.organization_id=? AND o.account_id=?`
	args := []any{query.ProjectID, query.OrganizationID, opaqueRef(query.AccountID), query.OrganizationID, query.AccountID}
	if query.Kind != "" {
		statement += ` AND o.object_kind=?`
		args = append(args, query.Kind)
	}
	if query.Status != "" {
		statement += ` AND o.status=?`
		args = append(args, query.Status)
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		statement += ` AND (o.display_name LIKE ? OR CONVERT(o.platform_object_id USING utf8mb4) LIKE ? OR JSON_UNQUOTE(JSON_EXTRACT(o.metadata_json,'$.product_id')) LIKE ? OR JSON_UNQUOTE(JSON_EXTRACT(o.metadata_json,'$.unique_product_id')) LIKE ?)`
		args = append(args, "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if cursor := strings.TrimSpace(query.Cursor); cursor != "" && query.SortBy == "" {
		statement += ` AND o.id>?`
		args = append(args, cursor)
	}
	sortOrder := strings.ToUpper(query.SortOrder)
	if sortOrder == "" {
		sortOrder = "DESC"
	}
	switch query.SortBy {
	case "created_at":
		statement += ` ORDER BY JSON_UNQUOTE(JSON_EXTRACT(o.metadata_json,'$.create_time')) ` + sortOrder + `,o.id ASC`
	case "ctr":
		statement += ` ORDER BY CASE WHEN COALESCE(m.impressions,0)>0 THEN COALESCE(m.clicks,0)/m.impressions ELSE -1 END ` + sortOrder + `,o.id ASC`
	case "conversions":
		statement += ` ORDER BY (m.data_through IS NOT NULL) DESC,COALESCE(m.conversions,0) ` + sortOrder + `,o.id ASC`
	default:
		statement += ` ORDER BY o.id ASC`
	}
	statement += ` LIMIT ?`
	args = append(args, limit)
	if query.SortBy != "" {
		statement += ` OFFSET ?`
		args = append(args, query.Offset)
	}
	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]PlatformObject, 0, limit)
	for rows.Next() {
		var value PlatformObject
		var metadata []byte
		var performance PlatformObjectPerformance
		var dataThrough sql.NullTime
		if err := rows.Scan(&value.ID, &value.OrganizationID, &value.AccountID, &value.Kind, &value.PlatformObjectID, &value.DisplayName, &value.Status, &metadata, &value.ObservedAt, &value.Version, &value.PreviewAvailable, &value.PreviewKind, &value.PreviewExpiresAt, &performance.SpendMinor, &performance.Impressions, &performance.Clicks, &performance.Conversions, &dataThrough); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &value.Metadata); err != nil {
			return nil, err
		}
		value.ProjectGranted = true
		performance.Available = dataThrough.Valid
		if performance.Impressions > 0 {
			performance.CTR = float64(performance.Clicks) / float64(performance.Impressions)
		}
		if dataThrough.Valid {
			performance.DataThrough = &dataThrough.Time
		}
		if value.Kind == PlatformObjectImageMaterial || value.Kind == PlatformObjectVideoMaterial || value.Kind == PlatformObjectAwemePhotoMaterial {
			value.Performance = &performance
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) GetPlatformObjectPreview(ctx context.Context, query PlatformObjectPreviewQuery) (PlatformObjectPreview, error) {
	if query.OrganizationID == "" || query.ProjectID == "" || query.AccountID == "" || query.ObjectID == "" || r.Cipher == nil {
		return PlatformObjectPreview{}, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return PlatformObjectPreview{}, err
	}
	var ciphertext []byte
	var keyVersion, kind string
	var objectKind PlatformObjectKind
	var expiresAt sql.NullTime
	err = db.QueryRowContext(ctx, `SELECT o.preview_url_ciphertext,o.preview_key_version,o.preview_kind,o.object_kind,o.preview_expires_at FROM connector_platform_objects o JOIN connector_platform_object_project_grants g ON g.organization_id=o.organization_id AND g.platform_object_id=o.id AND g.project_id=? AND g.status='active' WHERE o.organization_id=? AND o.account_id=? AND o.id=? AND o.status='active' AND o.preview_url_ciphertext IS NOT NULL`, query.ProjectID, query.OrganizationID, query.AccountID, query.ObjectID).Scan(&ciphertext, &keyVersion, &kind, &objectKind, &expiresAt)
	if err != nil {
		return PlatformObjectPreview{}, err
	}
	plaintext, err := r.Cipher.Decrypt(ciphertext, keyVersion)
	if err != nil {
		return PlatformObjectPreview{}, fmt.Errorf("decrypt platform object preview: %w", err)
	}
	value := PlatformObjectPreview{URL: string(plaintext), Kind: kind, ObjectKind: objectKind}
	if expiresAt.Valid {
		value.ExpiresAt = &expiresAt.Time
	}
	return value, nil
}

func (r MySQLRepository) updatePlatformObjectPreview(ctx context.Context, query PlatformObjectPreviewQuery, previewURL, previewKind string, expiresAt *time.Time, observedAt time.Time) error {
	if query.OrganizationID == "" || query.ProjectID == "" || query.AccountID == "" || query.ObjectID == "" || previewURL == "" || !validPreviewKind(previewKind) || observedAt.IsZero() || r.Cipher == nil {
		return ErrInvalidFact
	}
	ciphertext, keyVersion, err := r.Cipher.Encrypt([]byte(previewURL))
	if err != nil {
		return err
	}
	db, err := r.db()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE connector_platform_objects o JOIN connector_platform_object_project_grants g ON g.organization_id=o.organization_id AND g.platform_object_id=o.id AND g.project_id=? AND g.status='active' SET o.preview_url_ciphertext=?,o.preview_key_version=?,o.preview_kind=?,o.preview_expires_at=?,o.preview_observed_at=?,o.updated_at=? WHERE o.organization_id=? AND o.account_id=? AND o.id=? AND o.status='active'`, query.ProjectID, ciphertext, keyVersion, previewKind, expiresAt, observedAt, observedAt, query.OrganizationID, query.AccountID, query.ObjectID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r MySQLRepository) ReadPlatformObjectPreview(ctx context.Context, query PlatformObjectPreviewQuery) (PlatformObjectPreviewContent, error) {
	preview, err := r.GetPlatformObjectPreview(ctx, query)
	if err != nil || (preview.Kind != "image" && preview.Kind != "video_poster") {
		return PlatformObjectPreviewContent{}, ErrInvalidFact
	}
	target, err := url.Parse(preview.URL)
	if err != nil || !previewMediaHostAllowed(target.Hostname()) {
		return PlatformObjectPreviewContent{}, ErrInvalidFact
	}
	session, err := r.GetAccountSession(ctx, query.OrganizationID, query.AccountID)
	if err != nil || session.Status != AccountSessionReady {
		return PlatformObjectPreviewContent{}, err
	}
	cookie, err := r.Cipher.Decrypt(session.SessionCiphertext, session.SessionKeyVersion)
	if err != nil {
		return PlatformObjectPreviewContent{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, preview.URL, nil)
	if err != nil {
		return PlatformObjectPreviewContent{}, err
	}
	request.Header.Set("Cookie", string(cookie))
	request.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 5 || !previewMediaHostAllowed(request.URL.Hostname()) {
			return http.ErrUseLastResponse
		}
		request.Header.Set("Cookie", string(cookie))
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return PlatformObjectPreviewContent{}, err
	}
	defer response.Body.Close()
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if response.StatusCode < 200 || response.StatusCode >= 300 || !strings.HasPrefix(contentType, "image/") {
		return PlatformObjectPreviewContent{}, fmt.Errorf("platform preview upstream rejected request")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (12<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 12<<20 {
		return PlatformObjectPreviewContent{}, ErrInvalidFact
	}
	return PlatformObjectPreviewContent{ContentType: contentType, Data: data}, nil
}

func previewMediaHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, suffix := range []string{".oceanengine.com", ".byteadimg.com", ".byteimg.com", ".bytetos.com", ".douyinpic.com"} {
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}

func validPreviewKind(value string) bool {
	return value == "image" || value == "video_poster" || value == "landing_page"
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullableTime(present string, value time.Time) any {
	if present == "" {
		return nil
	}
	return value
}

func numericPlatformObjectID(value string) bool {
	if value == "" || len(value) > 191 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func validPlatformObjectID(kind PlatformObjectKind, value string) bool {
	if numericPlatformObjectID(value) {
		return true
	}
	if kind != PlatformObjectOptimizationTarget && kind != PlatformObjectIndustryCategory && kind != PlatformObjectBrand {
		return false
	}
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	return numericPlatformObjectID(value[1:])
}

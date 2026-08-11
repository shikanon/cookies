package assets

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type AssetRightsStatus string

const (
	AssetRightsUnverified AssetRightsStatus = "unverified"
	AssetRightsActive     AssetRightsStatus = "active"
	AssetRightsRevoked    AssetRightsStatus = "revoked"
)

type AssetUsePurpose string

const (
	AssetUsePreview       AssetUsePurpose = "preview"
	AssetUseTimelineSave  AssetUsePurpose = "timeline_save"
	AssetUseRenderPreview AssetUsePurpose = "render_preview"
	AssetUseRenderExport  AssetUsePurpose = "render_export"
	AssetUseSubmit        AssetUsePurpose = "submit"
)

type AssetRightsVersion struct {
	ID                    string                     `json:"id"`
	Version               int64                      `json:"version"`
	OrganizationID        contract.OrganizationID    `json:"organization_id"`
	ProjectID             contract.ProjectID         `json:"project_id"`
	AssetRef              contract.AssetVersionRef   `json:"asset_ref"`
	Source                string                     `json:"source"`
	RightsHolder          string                     `json:"rights_holder"`
	Status                AssetRightsStatus          `json:"status"`
	DerivativeWorkAllowed bool                       `json:"derivative_work_allowed"`
	GenerativeAIAllowed   bool                       `json:"generative_ai_allowed"`
	AllowedChannels       []string                   `json:"allowed_channels"`
	Territories           []string                   `json:"territories"`
	Purposes              []AssetUsePurpose          `json:"purposes"`
	ValidFrom             time.Time                  `json:"valid_from"`
	ValidUntil            *time.Time                 `json:"valid_until,omitempty"`
	RevokedAt             *time.Time                 `json:"revoked_at,omitempty"`
	AssertedBy            string                     `json:"asserted_by"`
	VerifiedBy            string                     `json:"verified_by,omitempty"`
	Evidence              []contract.AssetVersionRef `json:"evidence"`
	CreatedAt             time.Time                  `json:"created_at"`
}

type AssetRightsReader interface {
	GetCurrentAssetRights(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (AssetRightsVersion, error)
}

type AssetUseRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	AssetRef       contract.AssetVersionRef
	Purpose        AssetUsePurpose
	Channel        string
	Territory      string
}

type AssetUseDecision struct {
	Allowed       bool               `json:"allowed"`
	RightsID      string             `json:"rights_id,omitempty"`
	RightsVersion int64              `json:"rights_version,omitempty"`
	RightsStatus  AssetRightsStatus  `json:"rights_status"`
	Code          AssetUseDenialCode `json:"code,omitempty"`
}

type AssetUseDenialCode string

const (
	AssetRightsRevokedCode       AssetUseDenialCode = "ASSET_RIGHTS_REVOKED"
	AssetRightsExpiredCode       AssetUseDenialCode = "ASSET_RIGHTS_EXPIRED"
	AssetRightsNotYetValidCode   AssetUseDenialCode = "ASSET_RIGHTS_NOT_YET_VALID"
	AssetRightsUnverifiedCode    AssetUseDenialCode = "ASSET_RIGHTS_UNVERIFIED"
	AssetChannelNotAllowedCode   AssetUseDenialCode = "ASSET_CHANNEL_NOT_ALLOWED"
	AssetTerritoryNotAllowedCode AssetUseDenialCode = "ASSET_TERRITORY_NOT_ALLOWED"
	AssetPurposeNotAllowedCode   AssetUseDenialCode = "ASSET_PURPOSE_NOT_ALLOWED"
)

var ErrAssetUseDenied = errors.New("asset use denied")

type AssetUseDeniedError struct {
	Code AssetUseDenialCode
}

func (e AssetUseDeniedError) Error() string { return fmt.Sprintf("%s: %s", ErrAssetUseDenied, e.Code) }
func (e AssetUseDeniedError) Unwrap() error { return ErrAssetUseDenied }

type AssetUseAuthorizer interface {
	Authorize(context.Context, AssetUseRequest) (AssetUseDecision, error)
}

// AssetUsePolicy is the single authorization seam shared by preview, timeline
// persistence and render/submit boundaries. Rights are immutable versions;
// every decision reports the exact version it evaluated.
type AssetUsePolicy struct {
	Rights AssetRightsReader
	Now    func() time.Time
}

func (p AssetUsePolicy) Authorize(ctx context.Context, request AssetUseRequest) (AssetUseDecision, error) {
	if p.Rights == nil {
		return AssetUseDecision{}, fmt.Errorf("asset rights repository is required")
	}
	if request.OrganizationID == "" || request.ProjectID == "" || request.Purpose == "" {
		return AssetUseDecision{}, fmt.Errorf("asset use scope and purpose are required")
	}
	if err := request.AssetRef.Validate(); err != nil {
		return AssetUseDecision{}, err
	}
	rights, err := p.Rights.GetCurrentAssetRights(ctx, request.OrganizationID, request.ProjectID, request.AssetRef)
	if errors.Is(err, ErrNotFound) {
		decision := AssetUseDecision{RightsStatus: AssetRightsUnverified}
		if request.Purpose == AssetUsePreview || request.Purpose == AssetUseTimelineSave || request.Purpose == AssetUseRenderPreview {
			decision.Allowed = true
			return decision, nil
		}
		return deny(decision, AssetRightsUnverifiedCode)
	}
	if err != nil {
		return AssetUseDecision{}, err
	}
	decision := AssetUseDecision{RightsID: rights.ID, RightsVersion: rights.Version, RightsStatus: rights.Status}
	if rights.Status == AssetRightsRevoked || rights.RevokedAt != nil {
		return deny(decision, AssetRightsRevokedCode)
	}
	if rights.Status != AssetRightsActive {
		return deny(decision, AssetRightsUnverifiedCode)
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now()
	}
	if !rights.ValidFrom.IsZero() && now.Before(rights.ValidFrom) {
		return deny(decision, AssetRightsNotYetValidCode)
	}
	if rights.ValidUntil != nil && !now.Before(*rights.ValidUntil) {
		return deny(decision, AssetRightsExpiredCode)
	}
	if len(rights.Purposes) > 0 && !slices.Contains(rights.Purposes, request.Purpose) {
		return deny(decision, AssetPurposeNotAllowedCode)
	}
	if request.Channel != "" && !containsFold(rights.AllowedChannels, request.Channel) {
		return deny(decision, AssetChannelNotAllowedCode)
	}
	if request.Territory != "" && !containsFold(rights.Territories, request.Territory) {
		return deny(decision, AssetTerritoryNotAllowedCode)
	}
	decision.Allowed = true
	return decision, nil
}

func deny(decision AssetUseDecision, code AssetUseDenialCode) (AssetUseDecision, error) {
	decision.Code = code
	return decision, AssetUseDeniedError{Code: code}
}

func containsFold(values []string, target string) bool {
	if len(values) == 0 {
		return true
	}
	return slices.ContainsFunc(values, func(value string) bool { return strings.EqualFold(value, target) })
}

package assets

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type memoryRightsRepository struct {
	values map[contract.AssetID]AssetRightsVersion
}

func (r memoryRightsRepository) GetCurrentAssetRights(_ context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef) (AssetRightsVersion, error) {
	value, ok := r.values[ref.AssetID]
	if !ok || value.OrganizationID != org || value.ProjectID != project || value.AssetRef != ref {
		return AssetRightsVersion{}, ErrNotFound
	}
	return value, nil
}

func TestAssetUsePolicyRejectsRevokedExpiredAndTargetMismatch(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	ref := contract.AssetVersionRef{AssetID: "asset_1", Version: 1}
	base := AssetRightsVersion{
		ID: "rights_1", Version: 1, OrganizationID: "org_1", ProjectID: "project_1", AssetRef: ref,
		Status: AssetRightsActive, AllowedChannels: []string{"douyin"}, Territories: []string{"CN"},
		Purposes: []AssetUsePurpose{AssetUsePreview, AssetUseTimelineSave}, ValidFrom: now.Add(-time.Hour), ValidUntil: ptrTime(now.Add(time.Hour)),
	}
	tests := []struct {
		name    string
		mutate  func(*AssetRightsVersion)
		request AssetUseRequest
		code    AssetUseDenialCode
	}{
		{name: "revoked", mutate: func(v *AssetRightsVersion) { v.Status = AssetRightsRevoked }, request: AssetUseRequest{Purpose: AssetUsePreview}, code: AssetRightsRevokedCode},
		{name: "expired", mutate: func(v *AssetRightsVersion) { v.ValidUntil = ptrTime(now.Add(-time.Second)) }, request: AssetUseRequest{Purpose: AssetUsePreview}, code: AssetRightsExpiredCode},
		{name: "channel", mutate: func(*AssetRightsVersion) {}, request: AssetUseRequest{Purpose: AssetUsePreview, Channel: "xiaohongshu", Territory: "CN"}, code: AssetChannelNotAllowedCode},
		{name: "territory", mutate: func(*AssetRightsVersion) {}, request: AssetUseRequest{Purpose: AssetUsePreview, Channel: "douyin", Territory: "US"}, code: AssetTerritoryNotAllowedCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := base
			tt.mutate(&value)
			policy := AssetUsePolicy{Rights: memoryRightsRepository{values: map[contract.AssetID]AssetRightsVersion{ref.AssetID: value}}, Now: func() time.Time { return now }}
			tt.request.OrganizationID, tt.request.ProjectID, tt.request.AssetRef = "org_1", "project_1", ref
			_, err := policy.Authorize(context.Background(), tt.request)
			var denied AssetUseDeniedError
			if !errors.As(err, &denied) || denied.Code != tt.code {
				t.Fatalf("expected %s, got %#v", tt.code, err)
			}
		})
	}
}

func TestAssetUsePolicyKeepsUnknownEditableButBlocksDelivery(t *testing.T) {
	policy := AssetUsePolicy{Rights: memoryRightsRepository{values: map[contract.AssetID]AssetRightsVersion{}}}
	request := AssetUseRequest{OrganizationID: "org_1", ProjectID: "project_1", AssetRef: contract.AssetVersionRef{AssetID: "asset_1", Version: 1}, Purpose: AssetUseTimelineSave}
	decision, err := policy.Authorize(context.Background(), request)
	if err != nil || decision.RightsStatus != AssetRightsUnverified {
		t.Fatalf("expected editable unverified asset, got %#v, %v", decision, err)
	}
	request.Purpose = AssetUseRenderExport
	_, err = policy.Authorize(context.Background(), request)
	var denied AssetUseDeniedError
	if !errors.As(err, &denied) || denied.Code != AssetRightsUnverifiedCode {
		t.Fatalf("expected delivery denial, got %v", err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

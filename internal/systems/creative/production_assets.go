package creative

import (
	"context"

	platformassets "github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type ProductionAssetService interface {
	Get(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (platformassets.ProjectAsset, error)
	Preview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (platformassets.SignedRequest, error)
}

type AssetReadAdapter struct{ Assets ProductionAssetService }

func (a AssetReadAdapter) Resolve(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, refs []contract.AssetVersionRef) ([]ProductionAssetView, error) {
	items := make([]ProductionAssetView, 0, len(refs))
	for _, ref := range refs {
		asset, err := a.Assets.Get(ctx, actor, projectID, ref)
		if err != nil {
			return nil, err
		}
		view := ProductionAssetView{
			AssetRef: ref, MediaKind: productionMediaKind(asset.Asset.Kind), Availability: string(asset.Version.Status),
		}
		if asset.Version.MIMEType != "" {
			value := asset.Version.MIMEType
			view.MIMEType = &value
		}
		if asset.Version.WidthPixels > 0 {
			value := asset.Version.WidthPixels
			view.WidthPixels = &value
		}
		if asset.Version.HeightPixels > 0 {
			value := asset.Version.HeightPixels
			view.HeightPixels = &value
		}
		if asset.Version.DurationMS >= 0 && (asset.Asset.Kind == contract.AssetVideo || asset.Asset.Kind == contract.AssetAudio) {
			value := asset.Version.DurationMS
			view.DurationMS = &value
		}
		if asset.Version.Status == platformassets.AssetReady {
			preview, err := a.Assets.Preview(ctx, actor, projectID, ref)
			if err != nil {
				return nil, err
			}
			view.PreviewURL, view.PreviewExpiresAt = &preview.URL, &preview.ExpiresAt
		}
		items = append(items, view)
	}
	return items, nil
}

func productionMediaKind(kind contract.AssetKind) ProductionMediaKind {
	switch kind {
	case contract.AssetImage:
		return ProductionMediaImage
	case contract.AssetAudio:
		return ProductionMediaAudio
	default:
		return ProductionMediaVideo
	}
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"golang.org/x/image/draw"
)

// Bound image work across the complete paginated catalog, never just its first page.
func (r *deliveryFillingReader) images(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request delivery.FillingRequest, data *delivery.FillingContext) ([]provider.TextMessage, error) {
	if len(data.Choices["materials"])+len(data.Choices["product_images"]) > 24 {
		return nil, delivery.ErrFillingCatalogTooLarge
	}
	imageCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	messages := []provider.TextMessage{}
	failed := 0
	for _, field := range []string{"materials", "product_images"} {
		choices := data.Choices[field]
		type result struct {
			image  provider.TextImage
			source string
			err    error
		}
		results := make([]result, len(choices))
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for i, choice := range choices {
			wg.Add(1)
			go func(i int, choice delivery.FillingChoice) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-imageCtx.Done():
					results[i].err = imageCtx.Err()
					return
				}
				results[i].image, results[i].source, results[i].err = r.choiceImage(imageCtx, actor, projectID, request, choice)
			}(i, choice)
		}
		wg.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		valid := []delivery.FillingChoice{}
		for i, result := range results {
			if result.err != nil {
				failed++
				continue
			}
			choice := choices[i]
			if choice.Metadata == nil {
				choice.Metadata = map[string]any{}
			}
			choice.Metadata["visual_source"] = result.source
			choice.Metadata["image_sha256"] = fmt.Sprintf("%x", sha256.Sum256(result.image.Data))
			valid = append(valid, choice)
			messages = append(messages, provider.TextMessage{Role: provider.TextRoleUser, Content: fmt.Sprintf("素材候选 %s；标题：%s；图像来源：%s", choice.ID, choice.Label, result.source), Images: []provider.TextImage{result.image}})
		}
		data.Choices[field] = valid
	}
	if failed > 0 {
		data.Warnings = append(data.Warnings, fmt.Sprintf("%d 个素材图像读取失败，未参与选择。可刷新素材目录后重试。", failed))
	}
	if len(messages) > 0 {
		data.Sources = append(data.Sources, "素材图像及标题元数据")
		data.CanGenerateSellingPoints = true
	}
	return messages, nil
}

func (r *deliveryFillingReader) choiceImage(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request delivery.FillingRequest, choice delivery.FillingChoice) (provider.TextImage, string, error) {
	var ref delivery.StableReference
	if err := json.Unmarshal(choice.Value, &ref); err != nil {
		return provider.TextImage{}, "", err
	}
	var content io.ReadCloser
	source := "素材图片"
	if ref.Namespace == "cookies" {
		version, err := strconv.ParseInt(ref.Version, 10, 64)
		if err != nil {
			return provider.TextImage{}, "", err
		}
		assetRef := contract.AssetVersionRef{AssetID: contract.AssetID(ref.ID), Version: version}
		if ref.AuditAttributes["media_kind"] == "video" {
			if r.frames == nil {
				return provider.TextImage{}, "", fmt.Errorf("frame extraction unavailable")
			}
			frame, err := r.frames.ExtractFrame(ctx, media.FrameExtractionRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, SourceVideo: assetRef, TimestampMS: 0})
			if err != nil {
				return provider.TextImage{}, "", err
			}
			content = frame.Content
			source = "视频首帧（0ms）"
		} else {
			preview, _, err := r.assets.OpenPreview(ctx, actor, projectID, assetRef)
			if err != nil {
				return provider.TextImage{}, "", err
			}
			content = preview
		}
	} else {
		query := connector.PlatformObjectPreviewQuery{OrganizationID: string(actor.OrganizationID), ProjectID: string(projectID), AccountID: request.Ocean.Project.AccountReference.ID, ObjectID: ref.AuditAttributes["connector_platform_object_id"]}
		preview, err := r.catalog.ReadPlatformObjectPreview(ctx, query)
		if err != nil && r.previews != nil {
			if _, refreshErr := r.previews.RefreshPlatformObjectPreview(ctx, query); refreshErr == nil {
				preview, err = r.catalog.ReadPlatformObjectPreview(ctx, query)
			}
		}
		if err != nil {
			return provider.TextImage{}, "", err
		}
		content = io.NopCloser(bytes.NewReader(preview.Data))
		if ref.ObjectKind == "video_material" || ref.ObjectKind == "douyin_video" {
			source = "平台视频封面（不保证为首帧）"
		}
	}
	defer content.Close()
	raw, err := io.ReadAll(io.LimitReader(content, 12<<20+1))
	if err != nil || len(raw) > 12<<20 {
		return provider.TextImage{}, "", fmt.Errorf("invalid image size")
	}
	img, err := fillingThumbnail(raw)
	return img, source, err
}

func fillingThumbnail(raw []byte) (provider.TextImage, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return provider.TextImage{}, fmt.Errorf("invalid image dimensions")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return provider.TextImage{}, err
	}
	width, height := config.Width, config.Height
	if width > 768 || height > 768 {
		if width >= height {
			height = max(1, height*768/width)
			width = 768
		} else {
			width = max(1, width*768/height)
			height = 768
		}
	}
	thumb := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(thumb, thumb.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, thumb, &jpeg.Options{Quality: 85}); err != nil {
		return provider.TextImage{}, err
	}
	return provider.TextImage{MIMEType: "image/jpeg", Data: output.Bytes()}, nil
}

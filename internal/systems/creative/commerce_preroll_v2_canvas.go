package creative

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	stdDraw "image/draw"
	"image/png"
	"io"

	"github.com/shikanon/cookies/internal/platform/contract"
	xdraw "golang.org/x/image/draw"
)

func renderCommerceContainPNG(source io.Reader, width, height int) ([]byte, error) {
	if source == nil || width < 2 || height < 2 || width%2 != 0 || height%2 != 0 {
		return nil, fmt.Errorf("commerce image normalization dimensions are invalid")
	}
	decoded, _, err := image.Decode(source)
	if err != nil {
		return nil, fmt.Errorf("decode commerce anchor frame: %w", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() < 2 || bounds.Dy() < 2 {
		return nil, fmt.Errorf("commerce anchor frame is empty")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	background := averageCornerColor(decoded, bounds)
	stdDraw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: background}, image.Point{}, stdDraw.Src)
	scale := min(float64(width)/float64(bounds.Dx()), float64(height)/float64(bounds.Dy()))
	scaledWidth, scaledHeight := int(float64(bounds.Dx())*scale), int(float64(bounds.Dy())*scale)
	destination := image.Rect((width-scaledWidth)/2, (height-scaledHeight)/2, (width+scaledWidth)/2, (height+scaledHeight)/2)
	xdraw.CatmullRom.Scale(canvas, destination, decoded, bounds, stdDraw.Src, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode normalized commerce anchor frame: %w", err)
	}
	return output.Bytes(), nil
}

func averageCornerColor(source image.Image, bounds image.Rectangle) color.RGBA {
	points := []image.Point{
		{X: bounds.Min.X, Y: bounds.Min.Y},
		{X: bounds.Max.X - 1, Y: bounds.Min.Y},
		{X: bounds.Min.X, Y: bounds.Max.Y - 1},
		{X: bounds.Max.X - 1, Y: bounds.Max.Y - 1},
	}
	var red, green, blue uint32
	for _, point := range points {
		r, g, b, _ := source.At(point.X, point.Y).RGBA()
		red, green, blue = red+r, green+g, blue+b
	}
	return color.RGBA{R: uint8(red / uint32(len(points)) >> 8), G: uint8(green / uint32(len(points)) >> 8), B: uint8(blue / uint32(len(points)) >> 8), A: 255}
}

func (s Service) normalizeCommerceFirstFrameCandidate(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	candidate CommercePrerollV2FirstFrameCandidate,
	model ShortDramaModelCanvas,
	output ShortDramaOutputCanvas,
) (CommercePrerollV2FirstFrameCandidate, error) {
	if candidate.Asset == nil || s.ImageBaseAssets == nil || s.RenderedImages == nil {
		return candidate, fmt.Errorf("commerce first-frame normalization capability is unavailable")
	}
	readAndRender := func(asset contract.ProjectAssetRef, width, height int) ([]byte, error) {
		reader, err := s.ImageBaseAssets.OpenImage(ctx, actor, projectID, asset.AssetVersion)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return renderShortDramaCoverPNG(reader, width, height)
	}
	modelBytes, err := readAndRender(*candidate.Asset, model.Width, model.Height)
	if err != nil {
		return candidate, err
	}
	requestContext := contract.RequestContext{RequestID: "commerce-frame-" + candidate.ID, TraceID: taskID, Actor: actor}
	modelAsset, err := s.RenderedImages.IngestRenderedImage(ctx, requestContext, projectID, candidate.ID+"-model-canvas", bytes.NewReader(modelBytes), int64(len(modelBytes)), model.Width, model.Height, []contract.AssetVersionRef{candidate.Asset.AssetVersion}, nil)
	if err != nil {
		return candidate, err
	}
	candidate.ModelCanvasAsset = &modelAsset
	outputBytes, err := renderShortDramaCoverPNG(bytes.NewReader(modelBytes), output.Width, output.Height)
	if err != nil {
		return candidate, err
	}
	outputAsset, err := s.RenderedImages.IngestRenderedImage(ctx, requestContext, projectID, candidate.ID+"-output-canvas", bytes.NewReader(outputBytes), int64(len(outputBytes)), output.Width, output.Height, []contract.AssetVersionRef{modelAsset.AssetVersion}, nil)
	if err != nil {
		return candidate, err
	}
	candidate.OutputCanvasAsset = &outputAsset
	return candidate, nil
}

func (s Service) normalizeCommerceOpeningAnchor(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	frame CommercePrerollV2DerivedFrame,
	model ShortDramaModelCanvas,
) (CommercePrerollV2DerivedFrame, error) {
	if frame.Asset == nil || s.ImageBaseAssets == nil || s.RenderedImages == nil {
		return frame, fmt.Errorf("commerce opening-anchor normalization capability is unavailable")
	}
	reader, err := s.ImageBaseAssets.OpenImage(ctx, actor, projectID, frame.Asset.AssetVersion)
	if err != nil {
		return frame, err
	}
	contents, renderErr := renderCommerceContainPNG(reader, model.Width, model.Height)
	closeErr := reader.Close()
	if renderErr != nil {
		return frame, renderErr
	}
	if closeErr != nil {
		return frame, closeErr
	}
	renderHash, err := contract.CanonicalJSONHash(struct {
		DerivationID string `json:"derivation_id"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
	}{DerivationID: frame.DerivationID, Width: model.Width, Height: model.Height})
	if err != nil {
		return frame, err
	}
	if len(renderHash) > 24 {
		renderHash = renderHash[:24]
	}
	requestContext := contract.RequestContext{RequestID: "commerce-anchor-" + taskID, TraceID: taskID, Actor: actor}
	asset, err := s.RenderedImages.IngestRenderedImage(ctx, requestContext, projectID, "commerce-anchor-"+renderHash, bytes.NewReader(contents), int64(len(contents)), model.Width, model.Height, []contract.AssetVersionRef{frame.Asset.AssetVersion}, nil)
	if err != nil {
		return frame, err
	}
	frame.ModelCanvasAsset = &asset
	return frame, nil
}

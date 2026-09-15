package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/shikanon/cookies/internal/platform/media"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

func TestFillingThumbnailKeepsAspectRatioAndRejectsInvalidImage(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1600, 900))); err != nil {
		t.Fatal(err)
	}
	thumb, err := fillingThumbnail(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(thumb.Data))
	if err != nil || format != "jpeg" || config.Width != 768 || config.Height != 432 {
		t.Fatalf("thumbnail: %+v %s %v", config, format, err)
	}
	if _, err = fillingThumbnail([]byte("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}

func TestFillingVisualCatalogRequiresSearchAndDropsUnreadableCandidates(t *testing.T) {
	r := &deliveryFillingReader{}
	data := delivery.FillingContext{Choices: map[string][]delivery.FillingChoice{"materials": make([]delivery.FillingChoice, 25)}}
	_, err := r.images(context.Background(), contract.ActorContext{}, "p", delivery.FillingRequest{}, &data)
	if !errors.Is(err, delivery.ErrFillingCatalogTooLarge) {
		t.Fatal(err)
	}
	data.Choices["materials"] = []delivery.FillingChoice{{ID: "broken", Label: "broken", Value: json.RawMessage(`invalid`)}}
	messages, err := r.images(context.Background(), contract.ActorContext{}, "p", delivery.FillingRequest{}, &data)
	if err != nil || len(messages) != 0 || len(data.Choices["materials"]) != 0 || len(data.Warnings) == 0 {
		t.Fatalf("unreadable image retained: %+v %v", data, err)
	}
}

type fillingFrameStub struct {
	timestamp int64
	source    contract.AssetVersionRef
	data      []byte
}

func (s *fillingFrameStub) ExtractFrame(_ context.Context, r media.FrameExtractionRequest) (media.ExtractedFrame, error) {
	s.timestamp = r.TimestampMS
	s.source = r.SourceVideo
	return media.ExtractedFrame{Content: io.NopCloser(bytes.NewReader(s.data))}, nil
}
func TestFillingVideoUsesFirstFrameOfExactVersion(t *testing.T) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 20, 10))); err != nil {
		t.Fatal(err)
	}
	frames := &fillingFrameStub{timestamp: -1, data: raw.Bytes()}
	r := &deliveryFillingReader{frames: frames}
	img, source, err := r.choiceImage(context.Background(), contract.ActorContext{OrganizationID: "org"}, "project", delivery.FillingRequest{}, delivery.FillingChoice{Value: json.RawMessage(`{"namespace":"cookies","id":"asset","version":"7","audit_attributes":{"media_kind":"video"}}`)})
	if err != nil || frames.timestamp != 0 || frames.source.Version != 7 || source != "视频首帧（0ms）" || len(img.Data) == 0 {
		t.Fatalf("first frame: %+v %s %v", frames, source, err)
	}
}

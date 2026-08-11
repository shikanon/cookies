package creativeprovider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type shortDramaV2AssetOpenerStub struct{}

func (shortDramaV2AssetOpenerStub) OpenPreview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, assets.ObjectInfo, error) {
	return nil, assets.ObjectInfo{}, nil
}

func TestShortDramaAnalysisHTTPFailureIsClassified(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		status int
		want   error
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, want: creative.ErrShortDramaAnalysisProviderUnavailable},
		{name: "upstream unavailable", status: http.StatusBadGateway, want: creative.ErrShortDramaAnalysisProviderUnavailable},
		{name: "invalid multimodal request", status: http.StatusBadRequest, want: creative.ErrShortDramaAnalysisProviderRejected},
		{name: "credential rejected", status: http.StatusUnauthorized, want: creative.ErrShortDramaAnalysisProviderRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := shortDramaAnalysisHTTPFailure(test.status); !errors.Is(err, test.want) {
				t.Fatalf("shortDramaAnalysisHTTPFailure(%d) = %v, want category %v", test.status, err, test.want)
			}
		})
	}
}

func TestShortDramaAnalysisPayloadHonorsPromptJSONRoute(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"model": "seed", "messages": []any{}}
	if err := applyShortDramaTextRouteConstraints(payload, provider.GatewayRouteSnapshot{
		TextResponseMode:     provider.TextResponsePromptJSON,
		MaxOutputTokens:      8192,
		OutputTokenParameter: provider.TextOutputTokenParameterMaxTokens,
	}); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["response_format"]; exists {
		t.Fatal("prompt_json route must not receive an unsupported response_format")
	}
	if payload["max_tokens"] != 8192 {
		t.Fatalf("max_tokens = %#v, want 8192", payload["max_tokens"])
	}
}

type shortDramaV2TextRouteStub struct{}

func (shortDramaV2TextRouteStub) ResolveTextRoute(context.Context, contract.OrganizationID, string) (provider.GatewayRouteSnapshot, error) {
	return provider.GatewayRouteSnapshot{}, nil
}

type shortDramaV2CredentialStub struct{}

func (shortDramaV2CredentialStub) ResolveGatewayCredential(context.Context, string, int64) (string, error) {
	return "", nil
}

func TestNewShortDramaV2AnalyzerUsesNormalizedViralAnalyzerConfig(t *testing.T) {
	t.Parallel()

	analyzer, err := NewShortDramaV2Analyzer(ViralAnalyzerConfig{
		Assets: shortDramaV2AssetOpenerStub{}, Routes: shortDramaV2TextRouteStub{}, Credentials: shortDramaV2CredentialStub{},
		FFmpegPath: "ffmpeg", ModelAlias: "cookies.text.standard", PromptVersion: "short-drama-analysis/v1",
		ASR: ASRConfig{Endpoint: "https://example.com/asr"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if analyzer.config.Client == nil {
		t.Fatal("short drama analyzer did not inherit the normalized HTTP client")
	}
	if analyzer.config.WorkRoot != ".data/video-work" {
		t.Fatalf("short drama analyzer did not inherit the normalized work root: %q", analyzer.config.WorkRoot)
	}
}

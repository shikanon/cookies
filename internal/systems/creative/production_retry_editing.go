package creative

import (
	"context"
	"errors"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type EditingRenderRetryManager interface {
	RetryEditingRenderForProduction(context.Context, contract.RequestContext, contract.ProjectID, string, contract.IdempotencyKey) (EditingRenderJob, error)
}

// EditingRenderProductionRetryAdapter delegates to the existing editing owner
// command, which freezes the prior TimelineVersion and creates a new render job
// with retry_of. It never mutates the failed job.
type EditingRenderProductionRetryAdapter struct {
	Renders EditingRenderRetryManager
}

func (a EditingRenderProductionRetryAdapter) Supports(run ProductionSourceRun) bool {
	return a.Renders != nil && run.Summary.Ref.Source == ProductionSourceEditingRender && run.Summary.NormalizedStatus == ProductionFailed
}

func (a EditingRenderProductionRetryAdapter) Retry(ctx context.Context, command ProductionRetryContext) (ProductionRunRef, error) {
	job, err := a.Renders.RetryEditingRenderForProduction(ctx, command.RequestContext, command.ProjectID, command.Run.Summary.Ref.ID, command.IdempotencyKey)
	if errors.Is(err, ErrEditingRenderInputUnavailable) {
		return ProductionRunRef{}, ErrProductionInputAssetUnavailable
	}
	if err != nil {
		return ProductionRunRef{}, err
	}
	return ProductionRunRef{Source: ProductionSourceEditingRender, ID: job.ID}, nil
}

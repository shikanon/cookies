package delivery

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/browserautomation/webapi"
)

// CompileNext implements the Web API driver payload source. It derives the
// next pending staged create from the immutable plan version and the pending
// platform entity mappings of this run. The project stage precedes the
// promotion stages; promotions bind to the confirmed project platform ID.
func (p BrowserRpaAuthorityProvider) CompileNext(ctx context.Context, run browserautomation.BrowserRpaRun) (webapi.CompiledObject, bool, error) {
	authority := run.Authority
	repo, supported := p.Repository.(browserRpaStagedCreateRepository)
	if !supported {
		return webapi.CompiledObject{}, false, browserautomation.ErrInvalidContract
	}
	version, err := repo.GetPlanVersion(ctx, authority.OrganizationID, authority.ProjectID, authority.PlanID, authority.PlanVersion)
	if err != nil {
		return webapi.CompiledObject{}, false, mapBrowserRpaAuthorityError(err)
	}
	if version.PlatformConfiguration == nil || version.PlatformConfiguration.Payload.OceanEngine == nil || version.PlatformConfiguration.Payload.OceanEngine.Project == nil {
		return webapi.CompiledObject{}, false, browserautomation.ErrInvalidContract
	}
	ocean := version.PlatformConfiguration.Payload.OceanEngine

	if authority.Action == string(ControlledActionCreateProjectAndPromotions) {
		mapping, getErr := repo.GetPlatformEntityMappingByInternalObject(ctx, authority.OrganizationID, authority.ProjectID, authority.AccountReferenceID, "project", ocean.Project.ProjectDraftID)
		switch {
		case getErr == nil && mapping.Status == PlatformEntityMappingPending && mapping.BrowserRpaRunID == run.ID:
			object, compileErr := compileWebAPIProject(ocean.Project)
			if compileErr != nil {
				return webapi.CompiledObject{}, false, compileErr
			}
			return object, true, nil
		case getErr == nil:
			// Already confirmed or rebound elsewhere: continue to promotions.
		case errors.Is(getErr, ErrNotFound):
			// No mapping for this run: continue to promotions.
		default:
			return webapi.CompiledObject{}, false, mapBrowserRpaAuthorityError(getErr)
		}
	}

	for _, draft := range ocean.Promotions {
		mapping, getErr := repo.GetPlatformEntityMappingByInternalObject(ctx, authority.OrganizationID, authority.ProjectID, authority.AccountReferenceID, "promotion", draft.PromotionDraftID)
		if getErr != nil {
			if errors.Is(getErr, ErrNotFound) {
				continue
			}
			return webapi.CompiledObject{}, false, mapBrowserRpaAuthorityError(getErr)
		}
		if mapping.Status != PlatformEntityMappingPending || mapping.BrowserRpaRunID != run.ID {
			continue
		}
		projectPlatformID, bindErr := p.confirmedProjectPlatformID(ctx, repo, authority, ocean)
		if bindErr != nil {
			return webapi.CompiledObject{}, false, bindErr
		}
		object, compileErr := compileWebAPIPromotion(draft)
		if compileErr != nil {
			return webapi.CompiledObject{}, false, compileErr
		}
		object.DependsOnPlatformID = projectPlatformID
		return object, true, nil
	}
	return webapi.CompiledObject{}, false, nil
}

func (p BrowserRpaAuthorityProvider) confirmedProjectPlatformID(ctx context.Context, repo browserRpaStagedCreateRepository, authority browserautomation.AuthorityBinding, ocean *OceanEngineConfiguration) (string, error) {
	if ocean.Project != nil {
		mapping, getErr := repo.GetPlatformEntityMappingByInternalObject(ctx, authority.OrganizationID, authority.ProjectID, authority.AccountReferenceID, "project", ocean.Project.ProjectDraftID)
		if getErr == nil && mapping.Status == PlatformEntityMappingConfirmed && numericPlatformID(mapping.PlatformObjectID) {
			return mapping.PlatformObjectID, nil
		}
	}
	if authority.ParentPlatformProjectID != "" && numericPlatformID(authority.ParentPlatformProjectID) {
		return authority.ParentPlatformProjectID, nil
	}
	return "", fmt.Errorf("%w: promotion stage has no confirmed project binding", browserautomation.ErrInvalidContract)
}

func compileWebAPIProject(draft *OceanEngineProjectDraft) (webapi.CompiledObject, error) {
	object := webapi.CompiledObject{
		Kind:       "project",
		InternalID: draft.ProjectDraftID,
		Name:       draft.ProjectName,
		StartUnix:  draft.Schedule.StartAt.Unix(),
		EndUnix:    draft.Schedule.EndAt.Unix(),
	}
	if draft.OptimizationTargetReference == nil {
		return webapi.CompiledObject{}, fmt.Errorf("%w: project optimization target is absent", browserautomation.ErrInvalidContract)
	}
	externalAction := strings.TrimSpace(draft.OptimizationTargetReference.ID)
	if _, err := strconv.Atoi(externalAction); err != nil {
		return webapi.CompiledObject{}, fmt.Errorf("%w: project external_action is invalid", browserautomation.ErrInvalidContract)
	}
	object.ExternalAction = externalAction
	if draft.BudgetAndBidding.BudgetMode == OceanEngineBudgetModeDaily {
		// The captured create contract only covers the unlimited budget mode.
		// A daily budget write needs its own captured contract.
		return webapi.CompiledObject{}, fmt.Errorf("%w: project daily budget create contract is not captured", browserautomation.ErrInvalidContract)
	}
	if draft.BudgetAndBidding.BidMinor != nil && *draft.BudgetAndBidding.BidMinor > 0 {
		bid := float64(*draft.BudgetAndBidding.BidMinor) / 100
		object.BidYuan = &bid
	}
	if draft.MarketingProductReference != nil && draft.MarketingProductReference.State == ReferenceResolved {
		object.ProductReferenceID = draft.MarketingProductReference.ID
	}
	return object, nil
}

func compileWebAPIPromotion(draft OceanEnginePromotionDraft) (webapi.CompiledObject, error) {
	object := webapi.CompiledObject{
		Kind:       "promotion",
		InternalID: draft.PromotionDraftID,
		Name:       draft.PromotionName,
	}
	for _, reference := range draft.BaseMaterialReferences {
		if reference.State == ReferenceResolved && reference.ID != "" {
			object.MaterialReferenceIDs = append(object.MaterialReferenceIDs, reference.ID)
		}
	}
	return object, nil
}

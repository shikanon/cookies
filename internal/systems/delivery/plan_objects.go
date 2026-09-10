package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type PlanObjectAction struct {
	Kind          string                  `json:"kind"`
	InternalID    string                  `json:"internal_id"`
	Name          string                  `json:"name"`
	PlatformID    string                  `json:"platform_id,omitempty"`
	MappingID     string                  `json:"mapping_id,omitempty"`
	Action        string                  `json:"action"`
	ChangedFields []string                `json:"changed_fields"`
	Reason        string                  `json:"reason,omitempty"`
	Fields        []ObjectFieldPolicy     `json:"fields"`
	Differences   []ObjectFieldDifference `json:"differences,omitempty"`
}

type ObjectFieldDifference struct {
	Key    string `json:"key"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type ObjectFieldPolicy struct {
	Platform FieldAvailability `json:"platform"`
	Cookies  FieldAvailability `json:"cookies"`
	Key      string            `json:"key"`
	State    string            `json:"state"`
	Reason   string            `json:"reason"`
}

func objectFieldPolicies(kind string, bound bool, target any) []ObjectFieldPolicy {
	data, _ := json.Marshal(target)
	values := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &values)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]ObjectFieldPolicy, 0, len(keys))
	for _, key := range keys {
		field := ObjectFieldPolicy{Key: key, State: "editable", Reason: "未创建对象，可编辑本地草稿。"}
		if key == "project_draft_id" || key == "promotion_draft_id" || key == "draft_schema_version" {
			continue
		}
		if bound {
			field.State, field.Reason = "unverified", "此字段的已有对象编辑路径尚未校准，当前只读。"
			if key == "account_reference" || key == "marketing_purpose" || key == "marketing_scenario" || key == "delivery_mode" || key == "carrier" {
				field.State, field.Reason = "immutable", "Cookies 当前不支持修改已有对象的身份或创建路径。"
			}
			if kind == "promotion" && key == "budget_and_bidding" {
				field.State, field.Reason = "conditional", "仅日预算支持单独受控变更；须核对平台当前预算、绑定版本和适用条件。出价等字段尚未校准。"
			}
		}

		field.Platform = FieldAvailability{State: "unknown", Source: "none", Reason: "尚未读取此对象的当前平台编辑页。"}
		if !bound {
			field.Platform = FieldAvailability{State: "not_applicable", Source: "local_draft", Reason: "尚未创建，平台编辑限制不适用。"}
		}
		field.Cookies = FieldAvailability{State: field.State, Source: "cookies_workflow", Reason: field.Reason}
		fields = append(fields, field)
	}
	return fields
}

type PlanObjectPreview struct {
	PlanID  string             `json:"plan_id"`
	Version int64              `json:"version"`
	Objects []PlanObjectAction `json:"objects"`
}

func (s Service) PreviewPlanObjects(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string) (PlanObjectPreview, error) {
	plan, err := s.GetPlan(ctx, actor, projectID, planID)
	if err != nil {
		return PlanObjectPreview{}, err
	}
	return s.planObjectPreview(ctx, actor, projectID, plan)
}

func (s Service) planObjectPreview(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, plan DeliveryPlan) (PlanObjectPreview, error) {
	out := PlanObjectPreview{PlanID: plan.ID, Version: plan.Version, Objects: []PlanObjectAction{}}
	configuration := plan.CurrentVersion.PlatformConfiguration
	if configuration == nil || configuration.Payload.OceanEngine == nil || configuration.Payload.OceanEngine.Project == nil {
		return out, ErrLegacyConfigurationUnsupported
	}
	ocean := configuration.Payload.OceanEngine
	mappings, err := s.listPlatformEntityMappings(ctx, actor, projectID, ocean.Project.AccountReference.ID)
	if err != nil {
		return out, err
	}
	planMappings := make([]PlatformEntityMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.PlanID == plan.ID {
			planMappings = append(planMappings, mapping)
		}
	}
	mappings = planMappings
	if recovery, supported := s.Repository.(interface {
		CanRebindPendingPlatformEntityMapping(context.Context, PlatformEntityMapping, time.Time) (bool, error)
	}); supported {
		retained := make([]PlatformEntityMapping, 0, len(mappings))
		for _, mapping := range mappings {
			if mapping.PlanID == plan.ID && mapping.Status == PlatformEntityMappingPending {
				safe, checkErr := recovery.CanRebindPendingPlatformEntityMapping(ctx, mapping, s.now())
				if checkErr != nil {
					return out, checkErr
				}
				if safe {
					continue
				}
			}
			retained = append(retained, mapping)
		}
		mappings = retained
	}
	versions, err := s.Repository.ListPlanVersions(ctx, actor.OrganizationID, projectID, plan.ID)
	if err != nil {
		return out, err
	}
	authority, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return out, ErrUnsupportedConfigurationWorkflow
	}
	budgets, err := mappingBudgetBaselines(ctx, authority, mappings)
	if err != nil {
		return out, err
	}
	return buildPlanObjectPreview(plan, mappings, versions, budgets)
}

func buildPlanObjectPreview(plan DeliveryPlan, mappings []PlatformEntityMapping, versions []DeliveryPlanVersion, currentBudgets map[string]int64) (PlanObjectPreview, error) {
	out := PlanObjectPreview{PlanID: plan.ID, Version: plan.Version, Objects: []PlanObjectAction{}}
	ocean := plan.CurrentVersion.PlatformConfiguration.Payload.OceanEngine
	current := canonicalOceanConfiguration(ocean)
	used := map[string]bool{}
	appendObject := func(kind, id, name string, target any) error {
		item := PlanObjectAction{Kind: kind, InternalID: id, Name: name, Action: "create", ChangedFields: []string{}}
		var candidates []PlatformEntityMapping
		for _, mapping := range mappings {
			if mapping.PlanID == plan.ID && mapping.InternalObjectKind == kind && (kind == "project" || mapping.InternalObjectID == id) {
				candidates = append(candidates, mapping)
			}
		}
		if len(candidates) > 1 {
			return fmt.Errorf("%w: object %s has ambiguous platform bindings", ErrInvalidState, id)
		}
		if len(candidates) == 1 {
			mapping := candidates[0]
			used[mapping.ID] = true
			item.MappingID, item.PlatformID = mapping.ID, mapping.PlatformObjectID
			item.Action, item.Reason = "blocked", "平台绑定尚未确认，不能重复创建。"
			if mapping.Status == PlatformEntityMappingConfirmed && mapping.PlatformObjectID != "" {
				item.Reason = "缺少已确认对象的配置基线，请先核对平台对象。"
				for _, version := range versions {
					if version.PlatformConfiguration == nil || version.PlatformConfiguration.ConfigurationID != mapping.ConfigurationID {
						continue
					}
					baseline := canonicalOceanConfiguration(version.PlatformConfiguration.Payload.OceanEngine)
					if baseline == nil || baseline.Project == nil {
						continue
					}
					var previous any
					if kind == "project" {
						previous = baseline.Project
					} else {
						for _, promotion := range baseline.Promotions {
							if promotion.PromotionDraftID == id {
								snapshot := promotionComparison(promotion, version.PlatformConfiguration.Payload.OceanEngine)
								if amount, ok := currentBudgets[mapping.ID]; ok && snapshot.Budget != nil {
									budget := *snapshot.Budget
									budget.DailyBudgetMinor = amount
									snapshot.Budget = &budget
								}
								previous = snapshot
								break
							}
						}
					}
					if previous == nil {
						continue
					}
					item.Differences = objectFieldDifferences(previous, target)
					for _, difference := range item.Differences {
						item.ChangedFields = append(item.ChangedFields, difference.Key)
					}
					item.Action, item.Reason = "unchanged", "复用已确认的平台对象。"
					if len(item.ChangedFields) > 0 {
						item.Action, item.Reason = "update", "已有对象发生变更，请从对象编辑入口生成单独变更；不会重新创建。"
					}
					break
				}
			}
		}
		item.Fields = objectFieldPolicies(kind, len(candidates) != 0, target)
		out.Objects = append(out.Objects, item)
		return nil
	}
	if err := appendObject("project", ocean.Project.ProjectDraftID, ocean.Project.ProjectName, current.Project); err != nil {
		return out, err
	}
	for _, promotion := range current.Promotions {
		if err := appendObject("promotion", promotion.PromotionDraftID, promotion.PromotionName, promotionComparison(promotion, ocean)); err != nil {
			return out, err
		}
	}
	if out.Objects[0].Action == "create" {
		for _, mapping := range mappings {
			if mapping.PlanID == plan.ID && mapping.InternalObjectKind == "promotion" {
				out.Objects[0].Action = "blocked"
				out.Objects[0].Reason = "单元已有平台绑定，但缺少所属项目绑定。请核对项目，不能自动新建。"
				out.Objects[0].Fields = objectFieldPolicies("project", true, current.Project)
				break
			}
		}
	}
	for _, mapping := range mappings {
		if mapping.PlanID == plan.ID && !used[mapping.ID] {
			out.Objects = append(out.Objects, PlanObjectAction{Kind: mapping.InternalObjectKind, InternalID: mapping.InternalObjectID, PlatformID: mapping.PlatformObjectID, MappingID: mapping.ID, Action: "blocked", ChangedFields: []string{}, Reason: "历史绑定对象不在当前配置中。请核对对象身份；删除配置不会删除平台对象。"})
		}
	}
	return out, nil
}

func objectChangedFields(previous, target any) []string {
	fields := []string{}
	for _, difference := range objectFieldDifferences(previous, target) {
		fields = append(fields, difference.Key)
	}
	return fields
}

func objectFieldDifferences(previous, target any) []ObjectFieldDifference {
	decode := func(value any) map[string]any {
		data, _ := json.Marshal(value)
		out := map[string]any{}
		_ = json.Unmarshal(data, &out)
		delete(out, "project_draft_id")
		delete(out, "promotion_draft_id")
		return out
	}
	left, right := decode(previous), decode(target)
	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	fields := []ObjectFieldDifference{}
	for key := range keys {
		if !reflect.DeepEqual(left[key], right[key]) {
			fields = append(fields, ObjectFieldDifference{Key: key, Before: left[key], After: right[key]})
		}
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	return fields
}

type promotionObjectSnapshot struct {
	canonicalOceanEnginePromotion
	Budget *OceanEngineBudgetAndBidding `json:"budget_and_bidding,omitempty"`
}

func promotionComparison(promotion canonicalOceanEnginePromotion, ocean *OceanEngineConfiguration) promotionObjectSnapshot {
	var budget *OceanEngineBudgetAndBidding
	for _, value := range ocean.Promotions {
		if value.PromotionDraftID == promotion.PromotionDraftID {
			budget = value.BudgetAndBidding
			break
		}
	}
	return promotionObjectSnapshot{promotion, budget}
}

type mappingExecutionReader interface {
	GetControlledExecution(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledExecution, error)
	GetControlledChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledChangeSet, error)
}

func mappingBudgetBaselines(ctx context.Context, source mappingExecutionReader, mappings []PlatformEntityMapping) (map[string]int64, error) {
	budgets := map[string]int64{}
	for _, mapping := range mappings {
		if mapping.Status != PlatformEntityMappingConfirmed || mapping.InternalObjectKind != "promotion" || mapping.CurrentStateAction == "" {
			continue
		}
		execution, err := source.GetControlledExecution(ctx, mapping.OrganizationID, mapping.ProjectID, mapping.BusinessExecutionID)
		if err != nil {
			return nil, err
		}
		change, err := source.GetControlledChangeSet(ctx, mapping.OrganizationID, mapping.ProjectID, execution.ControlledChangeSetID)
		if err != nil {
			return nil, err
		}
		binding := change.Binding
		if change.Status != ControlledChangeSetExecuted || change.Action != mapping.CurrentStateAction || binding.TargetMappingID != mapping.ID || binding.TargetMappingVersion+1 != mapping.Version || binding.TargetPlatformObjectID != mapping.PlatformObjectID || binding.AccountReferenceID != mapping.AccountReferenceID {
			return nil, ErrApprovalContentMismatch
		}
		_, targetHash, err := binding.existingPromotionStateHashes(change.Action)
		if err != nil || targetHash != mapping.CurrentStateHash {
			return nil, ErrApprovalContentMismatch
		}
		switch {
		case binding.PromotionMutation != nil:
			budgets[mapping.ID] = binding.PromotionMutation.TargetDailyBudgetMinor
		case binding.PromotionControl != nil:
			budgets[mapping.ID] = binding.PromotionControl.CurrentDailyBudgetMinor
		case binding.PromotionRestart != nil:
			budgets[mapping.ID] = binding.PromotionRestart.ApprovedDailyBudgetMinor
		}
	}
	return budgets, nil
}

func validateBoundObjectEdits(previous, next *OceanEngineConfiguration, preview PlanObjectPreview) error {
	before, after := canonicalOceanConfiguration(previous), canonicalOceanConfiguration(next)
	for _, object := range preview.Objects {
		if object.MappingID == "" {
			continue
		}
		if object.Kind == "project" {
			if len(objectChangedFields(before.Project, after.Project)) != 0 {
				return contractFailure("OBJECT_EDIT_UNSUPPORTED", "project", "已有项目的修改路径尚未校准，不能从批量配置页改写。")
			}
			continue
		}
		var oldPromotion, nextPromotion *OceanEnginePromotionDraft
		for index := range previous.Promotions {
			if previous.Promotions[index].PromotionDraftID == object.InternalID {
				oldPromotion = &previous.Promotions[index]
			}
		}
		for index := range next.Promotions {
			if next.Promotions[index].PromotionDraftID == object.InternalID {
				nextPromotion = &next.Promotions[index]
			}
		}
		if oldPromotion == nil {
			continue
		}
		if nextPromotion == nil {
			return contractFailure("OBJECT_IDENTITY_REQUIRED", object.InternalID, "已绑定单元必须保留身份；删除草稿不能删除平台单元。")
		}
		var oldCanonical, nextCanonical canonicalOceanEnginePromotion
		for _, value := range before.Promotions {
			if value.PromotionDraftID == object.InternalID {
				oldCanonical = value
			}
		}
		for _, value := range after.Promotions {
			if value.PromotionDraftID == object.InternalID {
				nextCanonical = value
			}
		}
		if len(objectChangedFields(oldCanonical, nextCanonical)) != 0 {
			return contractFailure("OBJECT_EDIT_UNSUPPORTED", object.InternalID, "已有单元当前仅开放日预算变更；其他字段只读，等待校准。")
		}
		if reflect.DeepEqual(oldPromotion.BudgetAndBidding, nextPromotion.BudgetAndBidding) {
			continue
		}
		if oldPromotion.BudgetAndBidding == nil || nextPromotion.BudgetAndBidding == nil {
			return contractFailure("OBJECT_EDIT_UNSUPPORTED", object.InternalID, "缺少已有单元预算基线，不能修改预算。")
		}
		oldBudget, newBudget := *oldPromotion.BudgetAndBidding, *nextPromotion.BudgetAndBidding
		oldBudget.DailyBudgetMinor = newBudget.DailyBudgetMinor
		if !reflect.DeepEqual(oldBudget, newBudget) || object.PlatformID == "" {
			return contractFailure("OBJECT_EDIT_UNSUPPORTED", object.InternalID, "仅可调整已确认单元的日预算，不能同时修改出价或计费方式。")
		}
	}
	return nil
}

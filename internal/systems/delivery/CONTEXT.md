# Delivery domain context

Delivery owns the controlled transition from an immutable CreativePackage to an auditable advertising-platform change.

- **DeliveryPlan**: the approved intent, budget window, and immutable CreativePackage reference.
- **ChangeSet**: one versioned proposal to change a platform. It is the only object that can be preflighted, approved, executed, or rolled back.
- **Approval**: an immutable mock authority record binding one PlanVersion canonical hash and one ChangeSetVersion/action hash to an approver, 24-hour expiry, `execute_mock` scope, and budget snapshot. `delivery_change_sets.approved_by/approved_at` is only its compatibility projection. Execute and rollback advance lifecycle versions without changing the approved content version; those transitions must not be reported as approval content mismatches.
- **Execution**: one attempt to apply an approved ChangeSet.
- **Evidence**: the durable proof of what an Execution did. MVP evidence explicitly records local simulation and must never imply a real platform write.

Delivery does not own Creative content, Provider jobs, project identity, post-launch analysis, or reusable experience rules.

## Simulation and monitoring boundaries

The business monitoring page uses two independent paths.

- Prelaunch simulation reads one immutable `PlanVersion`. It runs the Mechanistic Simulation v0 model with an explicit prior set. It does not require an Execution.
- Post-launch inspection reads point-in-time Connector facts for the plan advertiser account. It persists only alerts from usable and fresh metric windows.

Connector quality gates fail closed. `quarantine`, stale, and incomplete metric windows return an explicit status. They do not create business alerts. Connector alerts use `source=connector` and `is_simulated=false`.

Decision policy v2 consumes the latest completed Mechanistic Simulation for the exact current `PlanVersion`. It selects the highest-probability scenario that has a recommendation draft. It creates three human-review action plans with low, medium, and high adjustment strength. Creative risk creates parallel project or promotion portfolio plans. It protects stable delivery objects and permits pruning only after a mature observation window. It does not rotate material on a stable object. Cost pressure and budget pacing can create bounded budget controls. Each candidate freezes its scenario, probability, focus, action, and action range into the decision hash. Historical decisions remain readable but do not replace the latest current-version decision in the business page.

Historical Connector calibration uses `delivery-calibration-case/v1`. A case can remain explicitly unbound from cookies Plan identity. The export uses a separate HMAC key, removes raw platform identity and free text, excludes post-launch diagnosis from prelaunch features, and treats operational retention or pause as an observed decision rather than a material-quality label.

## Historical insights consumer port

Delivery consumes read-only object and metric facts through its own
`InsightsConsumer` port. The port returns versioned account → project →
unit (platform promotion) → material mappings plus project-scoped `spend`, `impressions`,
`clicks`, and `conversions` facts. Each fact carries its window, granularity,
unit/currency, schema and definition versions, source (`mock`/`replay` or a
future `connector`), freshness, quality, and evidence references.

Historical fixtures and stored OutcomeSimulation records remain readable for audit and regression tests. They cannot create new business alerts. The retired `alerts:evaluate` endpoint returns HTTP 410; real monitoring uses `InspectConnectorAlerts` and its freshness, quality, and account boundaries.

Tour and legacy Recommendation writes are retired. The old approval and execution panels are read-only. Tests explicitly inject their deterministic execution adapter; missing production execution capability never generates simulated success.

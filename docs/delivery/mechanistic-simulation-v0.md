# Mechanistic Simulation v0

## Model boundary

This service answers one question: What can occur with the frozen configuration?

It does not estimate the causal effect of a configuration change.

The service does not read Connector history. It does not create remote actions.

## Input and prior contracts

The input binds one immutable PlanVersion, DeliveryIntent, OceanEngineConfiguration, and calibration Manifest.

The input includes budget, currency, schedule, objective, bid mode, delivery mode, targeting hash, and material references.

An unresolved dynamic reference returns `platform_pending`. The service does not use a display name as identity.

The caller supplies a versioned `SimulationPriorSet`. Each prior has a source, unit, scope, and uncertainty.

The service fails closed when a critical prior is absent. Code has no hidden performance default.

## Generation chain

The Monte Carlo chain is:

```text
review gate -> delivery gate -> budget utilization -> spend -> CPM
-> impressions -> CTR -> clicks -> CVR -> true conversions
-> observed conversions
```

All counts use integers. All money uses the configured minor currency unit.

Each sample derives CPM, CTR, CPC, CVR, and CPA from its atomic values.

A zero denominator makes a rate unavailable. It does not make the rate zero.

## Scenario and recommendation policy

Scenario labels are independent. One sample can match more than one label.

Tracking anomaly is `suspected` without independent tracking evidence.

Creative fatigue runs only when the caller supplies an enabled fatigue prior.

Recommendation drafts use rule or predictive-association language. They require human review.

The service does not create a ChangeSet, Approval, Execution, or Browser RPA Run.

## Safety and known limits

All results set `is_simulated=true` and `calibration_status=assumption_driven`.

The v0 model has no real-data calibration. It has no training or causal inference.

The research input `data/deep-research-report.md` was not present on the implementation branch.

The test prior fixture is artificial. It is not an industry benchmark.

## Legacy gap table

| Existing area | v0 treatment | Reason |
| --- | --- | --- |
| Canonical input hash and evidence references | Reuse | These types are stable and do not require Execution. |
| Integer metric counters | Reuse | They preserve atomic count and minor-unit semantics. |
| Fixed CPM, CTR, CVR, and revenue values | Replace in the new service | The new service accepts only an explicit prior set. |
| Single selected scenario | Replace | v0 reports independent multi-label probabilities. |
| Execution-bound create endpoint | Removed with the investor walkthrough | Existing records stay readable as audit history. |
| Legacy MetricSnapshot and Recommendation records | Keep unchanged | v0 stores a separate versioned result envelope. |
| Legacy alert rules | Do not reuse for prediction | v0 scenario detection reads only its Monte Carlo summary. |

The old endpoint remains a compatibility path. It is not the Mechanistic Simulation v0 entry.

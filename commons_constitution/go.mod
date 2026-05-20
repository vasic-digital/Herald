// Module commons_constitution is Herald's L1 governance layer per spec V3
// §42 (Constitution-flavor binding catalogue) + §44 (Foundation implementation
// contract).
//
// It exposes the Evaluator + Registry abstractions, the 12 event-class emit
// helpers under digital.vasic.herald.constitution.*, the BundleHash captureer
// for stable transition detection, the per-tenant per-rule ModeLadder, the
// ConstitutionStore transition gate, and an in-process EventBus shim whose
// interface mirrors the Catalogue-Check-recorded `digital.vasic.eventbus`
// shape so the swap-in at M2/M3 is mechanical.
//
// Per the Catalogue-Check (docs/catalogue-checks/HRD-018-foundation.md),
// the Evaluator framework + BundleHash captureer + ModeLadder semantics
// are bespoke to Helix governance (no-match) — all other Foundation
// concerns will be served by external Helix-stack modules at M2/M3.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_constitution

go 1.25.0

require (
	digital.vasic.database v0.0.0
	github.com/cloudevents/sdk-go/v2 v2.15.2
	github.com/google/uuid v1.6.0
)

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
)

replace github.com/vasic-digital/herald/commons => ../commons

replace digital.vasic.database => ../submodules/database

// Package constitution implements Herald's constitution-rule evaluator
// framework, the 12 event-class emit helpers, the bundle-hash captureer,
// the per-tenant per-rule mode-ladder, the constitution-state store with
// transition gate, and an in-process event bus shim.
//
// Reference: docs/specs/mvp/specification.V3.md §42, §44 and
//            docs/superpowers/specs/2026-05-20-foundation-design.md.
//
// Event namespace: digital.vasic.herald.constitution.<class>
// where <class> is one of the 12 entries enumerated in spec §42.2.
package constitution

// EventNamespace is the CloudEvents `ce-source` namespace under which
// every governance event emitted by this package is published.
const EventNamespace = "digital.vasic.herald.constitution"

// EventClasses enumerates the 12 governance event classes from spec §42.2.
// Each class has a typed emit helper in emit.go.
const (
	ClassGateFailed         = "gate.failed"
	ClassGateRecovered      = "gate.recovered"
	ClassPolicyViolation    = "policy.violation"
	ClassPolicyCleared      = "policy.cleared"
	ClassHostSafetyBreach   = "host.safety.breach"
	ClassRepoSafetyBreach   = "repo.safety.breach"
	ClassCredentialLeak     = "credential.leak"
	ClassBundleUpdated      = "bundle.updated"
	ClassBundleUpdateFailed = "bundle.update.failed"
	ClassReleaseGateBlocked = "release.gate.blocked"
	ClassSpecRevisionDrift  = "spec.revision.drift"
	ClassCatalogueMiss      = "catalogue.miss"
)

// AllClasses returns the closed set of the 12 event classes in declaration
// order. Useful for boot-time validation, metrics-label-cardinality bounds,
// and tests that must prove the package emits exactly these and no others.
func AllClasses() []string {
	return []string{
		ClassGateFailed,
		ClassGateRecovered,
		ClassPolicyViolation,
		ClassPolicyCleared,
		ClassHostSafetyBreach,
		ClassRepoSafetyBreach,
		ClassCredentialLeak,
		ClassBundleUpdated,
		ClassBundleUpdateFailed,
		ClassReleaseGateBlocked,
		ClassSpecRevisionDrift,
		ClassCatalogueMiss,
	}
}

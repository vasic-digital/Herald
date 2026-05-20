// Module commons is Herald's L0 foundation layer per spec V3 §10.
// It owns the CloudEvents envelope, Channel interface and its value
// types, Branding, error types, time / uuid helpers. Every other
// Herald module imports from here; commons imports nothing from other
// Herald modules — keeping it dependency-free at the Herald layer.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons

go 1.22

// commons is intentionally dependency-light at L0. Only stdlib + the
// canonical CloudEvents Go SDK (for type marshaling at the boundary).
require github.com/google/uuid v1.6.0

// CloudEvents SDK is a soft-required dependency — only the envelope
// converters in cloudevents.go reach into it. Adapters above L0 are
// free to consume cloudevents.Event directly.
require github.com/cloudevents/sdk-go/v2 v2.15.2

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
)

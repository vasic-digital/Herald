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

require github.com/spf13/cobra v1.10.2

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

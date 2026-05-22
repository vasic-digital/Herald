// Module commons is Herald's L0 foundation layer per spec V3 §10.
// It owns the CloudEvents envelope, Channel interface and its value
// types, Branding, error types, time / uuid helpers. Every other
// Herald module imports from here; commons imports nothing from other
// Herald modules — keeping it dependency-free at the Herald layer.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons

go 1.25.3

// commons is intentionally dependency-light at L0. Only stdlib + the
// canonical CloudEvents Go SDK (for type marshaling at the boundary).
require github.com/google/uuid v1.6.0

require (
	digital.vasic.http3 v0.0.0-00010101000000-000000000000
	digital.vasic.middleware v0.0.0-00010101000000-000000000000
	github.com/gin-gonic/gin v1.12.0
	github.com/quic-go/quic-go v0.59.0
	github.com/spf13/cobra v1.10.2
	github.com/vasic-digital/herald/commons_tls v0.0.0
)

// Wave 4a — http3 vendored as a Herald submodule at the pinned SHA
// 1d0df7b700436b70a361c3ba14d0520b070e7df9. The replace directive lets
// commons import digital.vasic.http3/pkg/server (the HTTP/3 listener
// wrapper) without going through a public Go proxy.
replace digital.vasic.http3 => ../submodules/http3

// Wave 4a Task 4 — middleware vendored as a Herald submodule at
// submodules/middleware. AltSvcMiddleware wraps
// digital.vasic.middleware/pkg/altsvc via .../pkg/gin.Wrap into a
// Gin handler. Future Wave 4a Task 5 will also use pkg/brotli.
replace digital.vasic.middleware => ../submodules/middleware

// commons_tls is a workspace sibling (also listed in go.work). The
// replace directive matches the pattern used by sherald/cherald/pherald
// for their cross-module references — keeps `go build ./commons/...`
// resolvable from the repo root regardless of whether the caller is
// inside or outside the workspace.
replace github.com/vasic-digital/herald/commons_tls => ../commons_tls

require (
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	golang.org/x/arch v0.22.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

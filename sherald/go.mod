module github.com/vasic-digital/herald/sherald

go 1.26.1

require (
	digital.vasic.toon v0.0.0-00010101000000-000000000000
	github.com/gin-gonic/gin v1.12.0
	github.com/google/uuid v1.6.0
	github.com/redis/go-redis/v9 v9.7.3
	github.com/spf13/cobra v1.10.2
	github.com/vasic-digital/herald/commons v0.0.0
	github.com/vasic-digital/herald/commons_auth v0.0.0
	github.com/vasic-digital/herald/commons_constitution v0.0.0
)

require (
	digital.vasic.database v0.0.0 // indirect
	digital.vasic.http3 v0.0.0-00010101000000-000000000000 // indirect
	digital.vasic.middleware v0.0.0-00010101000000-000000000000 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudevents/sdk-go/v2 v2.15.2 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/toon-format/toon-go v0.0.0-20251202084852-7ca0e27c4e8c // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	github.com/vasic-digital/herald/commons_storage v0.0.0-00010101000000-000000000000 // indirect
	github.com/vasic-digital/herald/commons_tls v0.0.0 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
	golang.org/x/arch v0.22.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/vasic-digital/herald/commons => ../commons

replace github.com/vasic-digital/herald/commons_auth => ../commons_auth

replace github.com/vasic-digital/herald/commons_constitution => ../commons_constitution

replace github.com/vasic-digital/herald/commons_storage => ../commons_storage

replace github.com/vasic-digital/herald/commons_infra => ../commons_infra

replace digital.vasic.cache => ../submodules/cache

replace digital.vasic.containers => ../containers

replace digital.vasic.database => ../submodules/database

replace digital.vasic.http3 => ../submodules/http3

replace digital.vasic.middleware => ../submodules/middleware

replace digital.vasic.toon => ../submodules/TOON

replace digital.vasic.background => ../submodules/background

replace digital.vasic.models => ../submodules/Models

replace github.com/vasic-digital/herald/commons_tls => ../commons_tls

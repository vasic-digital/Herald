module github.com/vasic-digital/herald/commons_messaging

go 1.25.3

require (
	digital.vasic.database v0.0.0
	github.com/google/uuid v1.6.0
	github.com/vasic-digital/herald/commons v0.0.0
	github.com/vasic-digital/herald/commons_storage v0.0.0
	gopkg.in/telebot.v3 v3.3.8
)

require (
	digital.vasic.background v0.0.0-00010101000000-000000000000 // indirect
	digital.vasic.cache v0.0.0 // indirect
	digital.vasic.containers v0.0.0 // indirect
	digital.vasic.models v0.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// commons_infra is required ONLY by persist_integration_test.go (build-tag
// gated). It's the canonical Quickstart-Boot helper used by every Foundation
// integration test in Herald.
require github.com/vasic-digital/herald/commons_infra v0.0.0

replace github.com/vasic-digital/herald/commons => ../commons

replace github.com/vasic-digital/herald/commons_storage => ../commons_storage

replace github.com/vasic-digital/herald/commons_infra => ../commons_infra

replace digital.vasic.background => ../submodules/background

replace digital.vasic.cache => ../submodules/cache

replace digital.vasic.containers => ../containers

replace digital.vasic.database => ../submodules/database

replace digital.vasic.models => ../submodules/Models

replace gopkg.in/telebot.v3 => ../submodules/telebot

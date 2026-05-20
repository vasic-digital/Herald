module github.com/vasic-digital/herald/commons_storage

go 1.25.0

require (
	digital.vasic.database v0.0.0
	github.com/google/uuid v1.6.0
)

// commons_infra is required ONLY by storage_integration_test.go (build-tag
// gated). Production code in this module MUST NOT import commons_infra —
// that would create a real import cycle (commons_infra imports
// commons_storage). Test packages are a separate compilation unit, so
// `package storage_test` importing infra is permitted.
require github.com/vasic-digital/herald/commons_infra v0.0.0

require (
	digital.vasic.cache v0.0.0 // indirect
	digital.vasic.containers v0.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace github.com/vasic-digital/herald/commons => ../commons

replace github.com/vasic-digital/herald/commons_infra => ../commons_infra

replace digital.vasic.cache => ../submodules/cache

replace digital.vasic.containers => ../containers

replace digital.vasic.database => ../submodules/database

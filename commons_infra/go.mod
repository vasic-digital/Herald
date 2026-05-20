// Module commons_infra is Herald's L1 on-demand-infra layer per spec V3 §44
// and Universal Constitution §11.4.76.
//
// It wraps `digital.vasic.containers/pkg/compose` so Foundation tests +
// the `pherald doctor` subcommand can bring Postgres + Redis + OTel up
// on-demand from the Herald quickstart compose file, satisfying the
// on-demand-infra invariant.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_infra

go 1.25.0

require (
	digital.vasic.cache v0.0.0
	digital.vasic.containers v0.0.0
	digital.vasic.database v0.0.0
	github.com/vasic-digital/herald/commons_storage v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace (
	digital.vasic.cache => ../submodules/cache
	digital.vasic.containers => ../containers
	digital.vasic.database => ../submodules/database
	github.com/vasic-digital/herald/commons_storage => ../commons_storage
)

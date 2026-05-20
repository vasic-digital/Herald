module github.com/vasic-digital/herald/commons_storage

go 1.25.0

require (
	digital.vasic.database v0.0.0
	github.com/google/uuid v1.6.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace github.com/vasic-digital/herald/commons => ../commons

replace digital.vasic.database => ../submodules/database

// Module commons_workable is the keystone of the ATMOSphere<->Herald
// workable-items integration. It mirrors ATMOSphere's SQLite SSoT
// schema (items / item_history / meta) so both projects share one DB,
// and provides a CRUD repo, a per-property change feed, and a tolerant
// parser for ATMOSphere's real Markdown tracker format.
//
// It uses the PURE-GO modernc.org/sqlite driver (no CGO) so it builds
// and tests anywhere without a C toolchain.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_workable

go 1.25.3

require modernc.org/sqlite v1.51.0

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

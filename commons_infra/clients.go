// Package infra — aggregator side of the on-demand-infra contract.
//
// QuickstartBoot owns the lifecycle of the Postgres + Redis + Queue
// clients that Herald's tests + the pherald doctor subcommand depend on.
// Boot()'s job is to bring the compose stack up; THIS file's job is to
// expose the resulting client handles to callers.
//
// Anti-bluff contract (§107 + §11.4.5):
//
//   - Pool / Queue / Redis getters MUST return a non-nil client only when
//     Up() has actually populated the corresponding field. The skeleton
//     here returns ErrNotBooted for every getter because Up() does not
//     yet wire any client (Task 2 of HRD-010 lands the pgx pool; Task 4
//     the Redis client; Task 5 the queue).
//
//   - Callers MUST check the returned error and abort. A test that
//     calls boot.Pool(), gets a nil + ErrNotBooted, and PASSes without
//     asserting is a bluff. The unit tests in clients_test.go pin
//     the contract via errors.Is(err, ErrNotBooted).
package infra

import (
	"errors"

	"digital.vasic.cache/pkg/redis"
	"digital.vasic.database/pkg/database"
)

// ErrNotBooted is returned by Pool/Queue/Redis when QuickstartBoot.Up()
// has not been called (or returned an error and never completed).
// Per §107 + §11.4.5, callers MUST check this error and abort — silently
// returning a nil client and PASSing the test on a no-op is a bluff.
var ErrNotBooted = errors.New("commons_infra: QuickstartBoot.Up() not called or failed; clients unavailable")

// Pool returns the live database connection pool wired by Up(), or
// ErrNotBooted if Up() has not been called.
//
// Returned type is digital.vasic.database/pkg/database.Database — the
// driver-agnostic interface; tests instantiate the postgres concrete
// implementation via pkg/postgres.New + Connect.
func (b *QuickstartBoot) Pool() (database.Database, error) {
	if b.pool == nil {
		return nil, ErrNotBooted
	}
	return b.pool, nil
}

// Queue returns the live task queue wired by Up(), or ErrNotBooted if
// Up() has not been called.
//
// Returned type is the Herald-local TaskQueue interface (see queue.go);
// when the digital.vasic.models submodule is incorporated under Herald
// (Task 5 of HRD-010), this becomes a thin alias for
// digital.vasic.background.TaskQueue per §11.4.74 extend-don't-reimplement.
func (b *QuickstartBoot) Queue() (TaskQueue, error) {
	if b.queue == nil {
		return nil, ErrNotBooted
	}
	return b.queue, nil
}

// Redis returns the live Redis client wired by Up(), or ErrNotBooted if
// Up() has not been called.
//
// Returned type is *digital.vasic.cache/pkg/redis.Client — the concrete
// client struct (not an interface) per the upstream API surface.
func (b *QuickstartBoot) Redis() (*redis.Client, error) {
	if b.redis == nil {
		return nil, ErrNotBooted
	}
	return b.redis, nil
}

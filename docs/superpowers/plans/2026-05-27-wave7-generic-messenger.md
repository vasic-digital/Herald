<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Wave 7 — Generic Messenger-Channel Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Genericize Herald's messenger-channel layer so Slack / Max / Email / Discord plug in without rewriting the inbound runtime or qaherald — extract the abstraction Telegram (Wave 6/6.5) implicitly defined, then prove it by landing Slack as the SECOND concrete implementation.

**Architecture:** A new `commons_messaging/channels` package defines a richer `channels.Channel` interface (extends the existing `commons.Channel` with inbound-runtime methods — `SendReply`, `BotSelfIdentity`, `DownloadAttachment`) plus an `init()`-based name→constructor registry. The Telegram adapter is refactored (pure, all existing tests stay green) to satisfy it. `pherald listen` becomes multi-channel: `HERALD_CHANNELS=tgram,slack` spins up N `Subscribe` goroutines feeding ONE `inbound.Dispatcher`. Attachments move to per-channel inbox subdirs (`~/.herald/inbox/<channel>/<sha256>.<ext>`). The bot self-filter generalizes via `BotSelfIdentity()`. Slack lands as a Socket Mode adapter (parity with Telegram long-poll). qaherald gains a Slack `MessengerClient` so the lifecycle test runs against Slack too. A §11.4.85 stress + chaos task exercises concurrent multi-channel inbound with fault injection.

**Tech Stack:** Go 1.25, `gopkg.in/telebot.v3` (vendored at `submodules/telebot`), `github.com/slack-go/slack` (NEW — vendored at `submodules/slack-go` per §11.4.74 catalogue-check + the spec "vendored SDKs as git submodule" convention), Cobra, `net/http`/`httptest` for hermetic round-trip tests, Postgres + Redis via `containers` submodule.

**Tag target:** `v0.6.0` (after T12). Tag is operator-gated on live `docs/qa/<run-id>/` evidence per §107.x.

**HRD block:** HRD-110 … HRD-121 (one per task). These are fresh numbers above the highest currently-assigned HRD-101 (Wave 6.5) and clear of the open HRD-081 / HRD-085..090 range. Each task opens its HRD in `docs/Issues.md` at task start and migrates it to `docs/Fixed.md` atomically at task close per V3 §8.3 + §11.4.19.

---

## Constitutional anchors (every task obeys these)

- **§107 / Helix §11.4 anti-bluff.** Each task's PASS MUST carry positive runtime evidence the user-visible behaviour works. A channel that "registers" but cannot actually `Send`/`Subscribe`/round-trip is a §107 defect. Metadata-only / compile-only / grep-only PASS is forbidden.
- **§107.x docs/qa evidence.** Each user-visible feature lands `docs/qa/HRD-NNN/` with a bidirectional transcript. Live transcripts SKIP-with-reason when creds are absent; hermetic `httptest` transcripts are committed verbatim otherwise.
- **§107.y working-tree quiescence.** Before every `git add`, grep the working tree for mutation markers (`MUTATED for paired`, `MUTATED W7-`, `// always pass`, `// MUTATION`). The Wave 7 mutation gate (T10) writes `.git/MUTATION_IN_PROGRESS` on entry and removes it on trap-exit; no commit proceeds while it is present.
- **§11.4.85 stress + chaos.** T11 is mandate-required: the multi-channel runtime ships with sustained-load + fault-injection suites whose PASS cites `docs/qa/<run-id>/stress_chaos/`.
- **§11.4.87 endless-loop autonomous work.** Tasks are parallelisable where non-contending; idle only while awaiting a result.
- **§11.4.88 background-push.** Release the commit-lock the instant `git commit` returns 0; push runs detached. Never gate local work on a remote round-trip.
- **§11.4.74 catalogue-check.** T6 records a catalogue-check line for `slack-go/slack` in the HRD-115 References cell (`reuse slack-go/slack@<sha>`).

## Locked design decisions (from the operator + brainstorm — do NOT relitigate)

1. The richer interface lives in a NEW package `commons_messaging/channels` (file `channels/channel.go`), NOT in `commons`. It embeds `commons.Channel` and adds the inbound-runtime methods. The existing `commons.Channel` in `commons/types.go` stays untouched (it is the spec §11.0 contract; widening it would ripple into every flavor's `serve` path).
2. Method set on `channels.Channel`: `Name()`, `Capabilities()` (both via embedded `commons.Channel`), plus `Subscribe(ctx, InboundHandler)`, `Send(ctx, OutboundMessage) (Receipt, error)` (via embed), and the NEW: `SendReply(ctx, recipient Recipient, body string, replyToID string, attachments []Attachment) (string, error)`, `BotSelfIdentity(ctx) (SelfIdentity, error)`, `DownloadAttachment(ctx, externalID string, mime string) (path string, sha256hex string, err error)`.
3. Per-channel inbox subdirs: `~/.herald/inbox/<channel>/<sha256>.<ext>` (was flat `~/.herald/inbox/<sha>.<ext>`).
4. Multi-channel `pherald listen`: `HERALD_CHANNELS=tgram,slack,email` env; per-channel namespaced env (`HERALD_TGRAM_*`, `HERALD_SLACK_*`, …). Each enabled channel runs its own `Subscribe` goroutine into the SAME `inbound.Dispatcher`. Default (env unset) = `tgram` only (Wave 6 behaviour preserved).
5. Channel registry: `commons_messaging/channels/registry.go` maps name→constructor; new channels register via `init()`.
6. Bot self-filter generalized: each channel's `BotSelfIdentity()` returns its native identity (tgram=username, slack=bot_user_id, email=From address). The inbound filter compares uniformly via `InboundEvent.Sender` carrying a self-identity match flag the adapter stamps.
7. Wave 7 ships Telegram-refactor + Slack adapter (SECOND impl). Max/Email are spec'd in §11.0/§32.2 but DEFERRED to Wave 8 (only the registry slots + env-name reservations land in Wave 7).
8. NO MTProto in Wave 7 (separate concern). Slack uses Socket Mode (WebSocket) for parity with Telegram long-poll.
9. The `*tgram.Adapter` `SendReply` signature today is `SendReply(ctx, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error)`. The generic `channels.Channel.SendReply` uses string IDs (`recipient.ChannelUserID`, `replyToID string`, returns `string`). The Telegram adapter keeps its native int64/int method AND gains a thin generic-signature wrapper (`SendReplyGeneric`) so the existing `inbound.Dispatcher` wiring and tests do not churn. The inbound dispatcher is migrated to the generic signature in T5.

---

## File Structure (created / modified across all tasks)

**New package `commons_messaging/channels/`:**
- `channel.go` — `channels.Channel` interface + `SelfIdentity` type (T1).
- `registry.go` — name→constructor registry, `Register`/`New`/`Names` (T2).
- `registry_test.go` — registry unit tests (T2).
- `inbox.go` — `InboxDir(channel)` + `WriteContentAddressed(channel, mime, reader)` per-channel content-addressing helper (T3).
- `inbox_test.go` — content-addressing + per-channel isolation tests (T3).
- `selffilter.go` — `IsSelfEcho(ev InboundEvent, self SelfIdentity)` generalized filter (T4).
- `selffilter_test.go` — self-filter matrix across channel identity kinds (T4).

**New Slack adapter `commons_messaging/channels/slack/`:**
- `slack.go` — `Adapter` struct + `New`/`NewWithBaseURL` constructors + `Name`/`Capabilities` + `init()` registration (T6).
- `subscribe.go` — Socket Mode `Subscribe` loop + `OnMessage`/`OnFile` handlers (T6).
- `send.go` — `Send` + `SendReply` (thread_ts) (T6).
- `attachments.go` — `DownloadAttachment` (files.info + url_private_download) (T6).
- `selfidentity.go` — `BotSelfIdentity` (auth.test) (T6).
- `slack_test.go`, `subscribe_test.go`, `send_test.go`, `attachments_test.go` — hermetic httptest round-trips (T6).

**Modified Telegram adapter `commons_messaging/channels/tgram/`:**
- `tgram.go` — add `init()` registration + `BotSelfIdentity` + `SendReplyGeneric` wrapper (T1, T2, T4).
- `attachments.go` — route through `channels.InboxDir(channel)` (T3).
- `subscribe.go` — stamp `InboundEvent.Sender` self-identity + use per-channel inbox (T3, T4).

**Modified pherald:**
- `pherald/cmd/pherald/listen.go` — multi-channel `HERALD_CHANNELS` fan-in (T5).
- `pherald/cmd/pherald/listen_test.go` — multi-channel hermetic test (T5).
- `pherald/internal/inbound/dispatcher.go` — generic `Replier` interface (T5).
- `pherald/cmd/pherald/stress_chaos_test.go` — NEW stress + chaos suite (T11).

**Modified qaherald:**
- `qaherald/internal/messenger/slack.go` — Slack `MessengerClient` impl (T7).
- `qaherald/internal/messenger/slack_test.go` — hermetic round-trip tests (T7).
- `qaherald/internal/messenger/builder.go` — channel-keyed `MessengerClient` builder (T7).
- `qaherald/internal/messenger/builder_test.go` — builder tests (T7).

**Docs / spec / gates:**
- `docs/specs/mvp/specification.V3.md` — §11.0 + §32.2 update; §43 row (T8).
- `docs/guides/MESSENGER_CHANNELS.md` — per-channel operator guide (T12).
- `scripts/e2e_bluff_hunt.sh` — E81..E88 multi-channel invariants (T9).
- `tests/test_wave7_mutation_meta.sh` — paired §1.1 mutation gate (T10).
- `docs/Issues.md` / `docs/Fixed.md` — HRD-110..HRD-121 lifecycle (every task).

---

## Pre-flight (run once before Task 1)

- [ ] **Confirm the worktree builds + tests green at HEAD**

Run:
```bash
cd /Users/milosvasic/Projects/Herald/.claude/worktrees/agent-a164e1fa368ad7a71
go build ./commons/... ./commons_messaging/... ./pherald/... ./qaherald/...
go test -race -count=1 ./commons_messaging/... ./pherald/... ./qaherald/...
```
Expected: all build + PASS. If anything FAILs at HEAD, STOP — fix at root cause before starting Wave 7 (a red baseline contaminates every paired-mutation proof downstream).

- [ ] **Confirm `go.work` lists commons_messaging + pherald + qaherald**

Run: `go work edit -json | grep -A1 commons_messaging`
Expected: the module is present. If `go.work` is missing (gitignored per §9.1), run `go work init && go work use ./...` first.

---

### Task 1 (HRD-110): Extract the `channels.Channel` interface

**Goal:** Define the richer interface in a new package and make the Telegram adapter satisfy it — a pure refactor. All existing tgram tests stay green; no behaviour changes.

**Files:**
- Create: `commons_messaging/channels/channel.go`
- Create: `commons_messaging/channels/channel_test.go`
- Modify: `commons_messaging/channels/tgram/tgram.go` (add `BotSelfIdentity` + `SendReplyGeneric`)

- [ ] **Step 1: Open the HRD**

Add this row to `docs/Issues.md` under `## Open` (status `in_progress`):
```
| HRD-110 | task | in_progress | high | Wave 7 T1 — extract commons_messaging/channels.Channel interface (SendReply/BotSelfIdentity/DownloadAttachment); tgram satisfies it (pure refactor) | 2026-05-27 | 2026-05-27 | spec V3 §11.0 + §32.2; Catalogue-Check: reuse (extends existing commons.Channel, 2026-05-27) |
```

- [ ] **Step 2: Write the failing test**

Create `commons_messaging/channels/channel_test.go` (imports: `testing`, `.../commons`, `.../commons_messaging/channels`, `.../commons_messaging/channels/tgram`):
```go
// TestTgramSatisfiesChannel: compile-time + identity assertion that tgram
// satisfies the richer interface. A pure-refactor regression (renamed method)
// breaks the build here.
func TestTgramSatisfiesChannel(t *testing.T) {
	var c channels.Channel = tgram.NewWithCreds("123:abc", "456")
	if c.Name() != string(commons.ChannelTelegram) { t.Fatalf("Name()=%q want %q", c.Name(), commons.ChannelTelegram) }
	if !c.Capabilities().Text { t.Fatal("tgram Capabilities().Text should be true") }
}
func TestSelfIdentityType(t *testing.T) { // pins the SelfIdentity shape T4 consumes
	id := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: "herald_bot"}
	if id.Kind != channels.IdentityUsername || id.Value != "herald_bot" { t.Fatalf("SelfIdentity mismatch: %+v", id) }
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./commons_messaging/channels/ -run TestTgramSatisfiesChannel -count=1`
Expected: FAIL — `package commons_messaging/channels is not in std` / undefined `channels.Channel`, `channels.SelfIdentity`, and `tgram` missing `BotSelfIdentity`/`SendReplyGeneric`.

- [ ] **Step 4: Create the interface package**

Create `commons_messaging/channels/channel.go`:
```go
// Package channels is the Wave 7 generic messenger-channel abstraction.
//
// It extends the spec §11.0 commons.Channel contract with the inbound-
// runtime methods that the Telegram adapter (Wave 6) implicitly defined:
// SendReply (quoted reply with attachment fan-out), BotSelfIdentity (the
// anti-echo-loop self-identity), and DownloadAttachment (content-addressed
// inbox write). Slack (T6), and later Max/Email (Wave 8), satisfy the same
// interface so pherald listen + qaherald never rewrite their core loops.
//
// Why a NEW package rather than widening commons.Channel: commons.Channel
// is the §11.0 contract every flavor's `serve` (outbound) path depends on;
// widening it would ripple into every flavor. The inbound runtime is a
// strictly richer need, so channels.Channel embeds commons.Channel and adds
// only the inbound methods.
package channels

import (
	"context"

	"github.com/vasic-digital/herald/commons"
)

// IdentityKind discriminates how an adapter computed its bot self-identity.
// The generalized self-filter (selffilter.go, T4) compares the right field
// per kind without knowing the concrete channel.
type IdentityKind string

const (
	// IdentityUsername — Telegram: bot.Me.Username (the @handle).
	IdentityUsername IdentityKind = "username"
	// IdentityUserID — Slack: the bot_user_id (e.g. "U01ABC").
	IdentityUserID IdentityKind = "user_id"
	// IdentityAddress — Email: the From address Herald sends as.
	IdentityAddress IdentityKind = "address"
)

// SelfIdentity is the channel-native identity the inbound runtime uses to
// drop self-echoes. The adapter returns it from BotSelfIdentity; the
// generalized filter (T4) compares InboundEvent.Sender against it.
type SelfIdentity struct {
	Kind  IdentityKind
	Value string // username (no @), user-id, or address — per Kind
}

// Channel is the Wave 7 richer adapter interface — embeds §11.0
// commons.Channel (Name/Capabilities/Send/Subscribe/HealthCheck) + the three
// inbound-runtime methods Telegram implicitly defined.
type Channel interface {
	commons.Channel

	// SendReply posts a reply quoting the message identified by replyToID,
	// then fans out each attachment as its own reply at the same thread depth.
	// recipient.ChannelUserID is the channel-native target (Telegram chat_id,
	// Slack channel id). replyToID == "" => no-reply sentinel (fresh message).
	// Returns the channel-native message id of the text reply.
	SendReply(ctx context.Context, recipient commons.Recipient, body string, replyToID string, attachments []commons.Attachment) (string, error)

	// BotSelfIdentity returns the channel-native bot identity. MUST cross the
	// wire on first call (getMe / auth.test); MAY cache. Empty Value is an
	// echo-loop hazard — Subscribe refuses to boot without a self-identity.
	BotSelfIdentity(ctx context.Context) (SelfIdentity, error)

	// DownloadAttachment streams the channel-hosted file externalID into
	// ~/.herald/inbox/<channel>/<sha256>.<ext> while hashing inline; returns
	// (finalPath, sha256Hex, error). Idempotent (content == proof of presence).
	DownloadAttachment(ctx context.Context, externalID string, mime string) (string, string, error)
}
```

- [ ] **Step 5: Add the tgram methods that satisfy the new interface**

In `commons_messaging/channels/tgram/tgram.go`, add these methods (after `Capabilities`). The package-level `DownloadAttachment` func already exists (Wave 6); the new METHOD adapts it (T3 migrates the package func to per-channel inbox; this method is the stable interface seam):
```go
// BotSelfIdentity returns the bot's @username via getMe (ensureBot IS the live
// roundtrip — telebot.NewBot dispatches getMe synchronously). Empty username =
// §107 echo-loop hazard; inbound runtime refuses to boot on it.
func (a *Adapter) BotSelfIdentity(ctx context.Context) (channels.SelfIdentity, error) {
	_ = ctx // telebot.v3 getMe is not ctx-aware
	if err := a.ensureBot(); err != nil { return channels.SelfIdentity{}, fmt.Errorf("tgram.BotSelfIdentity: %w", err) }
	if a.bot == nil || a.bot.Me == nil || a.bot.Me.Username == "" {
		return channels.SelfIdentity{}, errors.New("tgram.BotSelfIdentity: getMe empty username (echo-loop hazard)")
	}
	return channels.SelfIdentity{Kind: channels.IdentityUsername, Value: a.bot.Me.Username}, nil
}

// SendReplyGeneric adapts the string-typed channels.Channel.SendReply to the
// native int64/int Telegram method. recipient.ChannelUserID = decimal chat_id;
// replyToID = decimal message_id ("" => no reply). Returns decimal message_id.
func (a *Adapter) SendReplyGeneric(ctx context.Context, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error) {
	chatID, err := strconv.ParseInt(recipient.ChannelUserID, 10, 64)
	if err != nil { return "", fmt.Errorf("tgram.SendReplyGeneric: chatID %q not numeric: %w", recipient.ChannelUserID, err) }
	replyTo := 0
	if replyToID != "" { if r, perr := strconv.Atoi(replyToID); perr == nil { replyTo = r } }
	id, err := a.SendReply(ctx, chatID, body, replyTo, attachments)
	if err != nil { return "", err }
	return strconv.Itoa(id), nil
}

// DownloadAttachment (method) satisfies channels.Channel — routes the package-
// level DownloadAttachment through ensureBot's live bot.
func (a *Adapter) DownloadAttachment(ctx context.Context, externalID, mime string) (string, string, error) {
	if err := a.ensureBot(); err != nil { return "", "", fmt.Errorf("tgram.DownloadAttachment: %w", err) }
	return DownloadAttachment(ctx, a.bot, externalID, mime)
}
```
Add `"github.com/vasic-digital/herald/commons_messaging/channels"` + `"strconv"` to tgram.go's imports (`errors`, `fmt` are already present).

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./commons_messaging/channels/ -run 'TestTgramSatisfiesChannel|TestSelfIdentityType' -count=1`
Expected: PASS.

- [ ] **Step 7: Run the full tgram suite to prove the refactor is pure**

Run: `go test -race -count=1 ./commons_messaging/channels/tgram/...`
Expected: PASS — every Wave 6 / 6.5 test stays green. This is the §107 anti-bluff anchor for T1: a refactor that broke a behaviour would surface here.

- [ ] **Step 8: §107.y quiescence check + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass|MUTATED for paired' commons_messaging/channels/ || echo "clean"`
Expected: `clean`.
```bash
git add commons_messaging/channels/channel.go commons_messaging/channels/channel_test.go \
        commons_messaging/channels/tgram/tgram.go docs/Issues.md
git commit -m "$(cat <<'EOF'
Wave 7 T1 (HRD-110): extract commons_messaging/channels.Channel interface

New channels.Channel embeds commons.Channel (§11.0) + adds the inbound-
runtime methods Telegram implicitly defined: SendReply (string-typed),
BotSelfIdentity (SelfIdentity), DownloadAttachment. tgram.Adapter gains
BotSelfIdentity + SendReplyGeneric + DownloadAttachment wrappers — pure
refactor, all Wave 6/6.5 tgram tests stay green.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```
Do NOT migrate HRD-110 to Fixed yet — it closes at T1 completion review (the row stays `in_progress` until the reviewer confirms the pure-refactor invariant). For a solo-agent run, migrate it to `docs/Fixed.md` now and note "live evidence via T9 e2e invariant E81".

---

### Task 2 (HRD-111): Channel registry + `init()`-based registration

**Goal:** A name→constructor registry so `pherald listen` resolves channels by name and new adapters self-register via `init()`.

**Files:**
- Create: `commons_messaging/channels/registry.go`
- Create: `commons_messaging/channels/registry_test.go`
- Modify: `commons_messaging/channels/tgram/tgram.go` (add `init()` registration)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-111 | task | in_progress | high | Wave 7 T2 — channel registry (name→constructor) + init()-based registration | 2026-05-27 | 2026-05-27 | spec V3 §11.0; mirrors qaherald scenario.Register pattern; Catalogue-Check: reuse (internal pattern, 2026-05-27) |
```

- [ ] **Step 2: Write the failing test**

Create `commons_messaging/channels/registry_test.go`. Imports: `context`, `errors`, `testing`, `.../commons`, `.../commons_messaging/channels`, AND a blank import `_ ".../commons_messaging/channels/tgram"` (forces tgram's `init()` for `TestTgramRegisteredViaInit`). Both T1 `channel_test.go` and this file are `package channels_test`, so the `fakeChannel` stub below is declared ONCE here and shared. It satisfies all 8 `channels.Channel` methods (Name/Capabilities/Send/Subscribe/HealthCheck/SendReply/BotSelfIdentity/DownloadAttachment) with trivial returns:
```go
type fakeChannel struct{ name string }
func (f *fakeChannel) Name() string                       { return f.name }
func (f *fakeChannel) Capabilities() commons.Capabilities { return commons.Capabilities{Text: true} }
func (f *fakeChannel) Send(context.Context, commons.OutboundMessage) (commons.Receipt, error) { return commons.Receipt{}, nil }
func (f *fakeChannel) Subscribe(context.Context, commons.InboundHandler) error                { return nil }
func (f *fakeChannel) HealthCheck(context.Context) error                                      { return nil }
func (f *fakeChannel) SendReply(context.Context, commons.Recipient, string, string, []commons.Attachment) (string, error) { return "", nil }
func (f *fakeChannel) BotSelfIdentity(context.Context) (channels.SelfIdentity, error)         { return channels.SelfIdentity{}, nil }
func (f *fakeChannel) DownloadAttachment(context.Context, string, string) (string, string, error) { return "", "", nil }

func TestRegistryResolvesRegisteredChannel(t *testing.T) {
	channels.Register("fake-rt", func(channels.Config) (channels.Channel, error) { return &fakeChannel{name: "fake-rt"}, nil })
	c, err := channels.New("fake-rt", channels.Config{})
	if err != nil { t.Fatalf("New(fake-rt): %v", err) }
	if c.Name() != "fake-rt" { t.Fatalf("Name()=%q want fake-rt", c.Name()) }
}
func TestRegistryUnknownChannelErrors(t *testing.T) {
	_, err := channels.New("does-not-exist", channels.Config{})
	if err == nil || !errors.Is(err, channels.ErrUnknownChannel) { t.Fatalf("err=%v want ErrUnknownChannel", err) }
}
func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() { if recover() == nil { t.Fatal("duplicate Register should panic") } }()
	channels.Register("dup-rt", func(channels.Config) (channels.Channel, error) { return nil, nil })
	channels.Register("dup-rt", func(channels.Config) (channels.Channel, error) { return nil, nil })
}
func TestTgramRegisteredViaInit(t *testing.T) { // blank import above triggers init()
	for _, n := range channels.Names() { if n == string(commons.ChannelTelegram) { return } }
	t.Fatalf("tgram not registered via init(); Names()=%v", channels.Names())
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./commons_messaging/channels/ -run TestRegistry -count=1`
Expected: FAIL — undefined `channels.Register`, `channels.New`, `channels.Config`, `channels.Names`, `channels.ErrUnknownChannel`.

- [ ] **Step 4: Write the registry**

Create `commons_messaging/channels/registry.go`:
```go
package channels

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrUnknownChannel is returned by New when no constructor is registered for
// the requested channel name. Callers (pherald listen) MUST surface it
// explicitly — a silent no-op channel is a §107 PASS-bluff.
var ErrUnknownChannel = errors.New("channels: unknown channel")

// Config is the per-channel constructor input — channel-agnostic; an adapter
// reads only the fields it understands.
type Config struct {
	Channel  string            // name being constructed ("tgram", "slack")
	Token    string            // primary credential (Telegram token, Slack xoxb-)
	AppToken string            // secondary (Slack xapp- app-level token for Socket Mode)
	Target   string            // default outbound dest (Telegram chat_id, Slack channel id, email)
	BaseURL  string            // httptest seam; "" => live endpoint
	Extra    map[string]string // channel-specific (e.g. email IMAP host/port)
}

// Constructor builds a Channel from a Config. Registered via init().
type Constructor func(cfg Config) (Channel, error)

var (
	mu       sync.RWMutex
	registry = map[string]Constructor{}
)

// Register installs ctor under name. Panics on duplicate — a typo in two
// adapters' init() would silently shadow one (a bluff class this framework
// forbids; mirrors qaherald scenario.Register).
func Register(name string, ctor Constructor) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[name]; ok { panic(fmt.Sprintf("channels: duplicate registration for %q", name)) }
	registry[name] = ctor
}

// New constructs the channel registered under name (wrapped ErrUnknownChannel
// when none).
func New(name string, cfg Config) (Channel, error) {
	mu.RLock()
	ctor, ok := registry[name]
	mu.RUnlock()
	if !ok { return nil, fmt.Errorf("%w: %q (registered: %v)", ErrUnknownChannel, name, Names()) }
	cfg.Channel = name
	return ctor(cfg)
}

// Names returns every registered channel name, alphabetical.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry { out = append(out, n) }
	sort.Strings(out)
	return out
}
```

- [ ] **Step 5: Register tgram via `init()`**

Append to `commons_messaging/channels/tgram/tgram.go`:
```go
// init registers the Telegram adapter with the channels registry (Wave 7 T2)
// so `pherald listen` can resolve "tgram" by name. cfg.Token is the bot
// token; cfg.Target is the chat_id; cfg.BaseURL is the httptest seam.
func init() {
	channels.Register(string(commons.ChannelTelegram), func(cfg channels.Config) (channels.Channel, error) {
		if cfg.BaseURL != "" {
			return NewAdapterWithBaseURL(cfg.Token, cfg.Target, cfg.BaseURL), nil
		}
		return NewWithCreds(cfg.Token, cfg.Target), nil
	})
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./commons_messaging/channels/ -run 'TestRegistry|TestTgramRegisteredViaInit' -count=1`
Expected: PASS.

- [ ] **Step 7: Quiescence + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass' commons_messaging/channels/ || echo clean`
```bash
git add commons_messaging/channels/registry.go commons_messaging/channels/registry_test.go \
        commons_messaging/channels/tgram/tgram.go docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T2 (HRD-111): channel registry + init()-based registration

channels.Register/New/Names map channel-name → Constructor; tgram self-
registers via init(). New returns wrapped ErrUnknownChannel for unknown
names (no silent no-op). Duplicate Register panics. Mirrors the qaherald
scenario.Register pattern.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3 (HRD-112): Per-channel inbox subdirs

**Goal:** Move content-addressed attachment storage from flat `~/.herald/inbox/<sha>.<ext>` to per-channel `~/.herald/inbox/<channel>/<sha>.<ext>`, with a shared helper both adapters use.

**Files:**
- Create: `commons_messaging/channels/inbox.go`
- Create: `commons_messaging/channels/inbox_test.go`
- Modify: `commons_messaging/channels/tgram/attachments.go` (route through the helper)
- Modify: `commons_messaging/channels/tgram/attachments_test.go` (assert the new path shape)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-112 | task | in_progress | middle | Wave 7 T3 — per-channel inbox subdirs ~/.herald/inbox/<channel>/<sha256>.<ext> | 2026-05-27 | 2026-05-27 | spec V3 §32; Catalogue-Check: reuse (internal, 2026-05-27) |
```

- [ ] **Step 2: Write the failing test**

Create `commons_messaging/channels/inbox_test.go` (imports: `bytes`, `io`, `os`, `path/filepath`, `strings`, `testing`, `.../commons_messaging/channels`; both tests `t.Setenv("HOME", t.TempDir())`):
```go
func TestInboxDirIsPerChannel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tg, err := channels.InboxDir("tgram")
	if err != nil { t.Fatalf("InboxDir(tgram): %v", err) }
	if !strings.HasSuffix(tg, filepath.Join(".herald", "inbox", "tgram")) { t.Fatalf("tgram inbox=%q want .../inbox/tgram", tg) }
	sl, _ := channels.InboxDir("slack")
	if tg == sl { t.Fatal("tgram and slack inbox dirs must differ") }
}

func TestWriteContentAddressedHashesAndIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	payload := []byte("hello wave7 attachment")
	path1, sum1, err := channels.WriteContentAddressed("slack", "text/plain", io.NopCloser(bytes.NewReader(payload)))
	if err != nil { t.Fatalf("WriteContentAddressed: %v", err) }
	if !strings.Contains(path1, filepath.Join("inbox", "slack")) { t.Fatalf("path %q not under inbox/slack", path1) }
	if !strings.HasSuffix(path1, sum1+".txt") { t.Fatalf("path %q not <sha>.txt (sum=%s)", path1, sum1) }
	if got, _ := os.ReadFile(path1); !bytes.Equal(got, payload) { t.Fatalf("on-disk bytes mismatch") }
	// Idempotent: 2nd write same content → same path, no .part residue.
	path2, sum2, err := channels.WriteContentAddressed("slack", "text/plain", io.NopCloser(bytes.NewReader(payload)))
	if err != nil || path2 != path1 || sum2 != sum1 { t.Fatalf("idempotency: (%q,%q)!=(%q,%q) err=%v", path2, sum2, path1, sum1, err) }
	entries, _ := os.ReadDir(filepath.Dir(path1))
	for _, e := range entries { if strings.HasSuffix(e.Name(), ".part") { t.Fatalf("leftover .part %q after idempotent write", e.Name()) } }
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./commons_messaging/channels/ -run 'TestInboxDir|TestWriteContentAddressed' -count=1`
Expected: FAIL — undefined `channels.InboxDir`, `channels.WriteContentAddressed`.

- [ ] **Step 4: Write the inbox helper**

Create `commons_messaging/channels/inbox.go`:
```go
package channels

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// InboxDir returns ~/.herald/inbox/<channel>/, creating it (0700) if absent.
// Per-channel isolation lets the same sha256 from two channels coexist + makes
// the on-disk forensic trail self-describing by channel.
func InboxDir(channel string) (string, error) {
	if channel == "" { return "", fmt.Errorf("channels.InboxDir: empty channel") }
	home, err := os.UserHomeDir()
	if err != nil { return "", fmt.Errorf("channels.InboxDir: resolve home: %w", err) }
	dir := filepath.Join(home, ".herald", "inbox", channel)
	if err := os.MkdirAll(dir, 0o700); err != nil { return "", fmt.Errorf("channels.InboxDir: mkdir %s: %w", dir, err) }
	return dir, nil
}

// WriteContentAddressed streams r into <inbox>/<sha256>.<ext> while hashing
// inline (io.MultiWriter — never buffers the full payload), then atomically
// renames into place. Returns (finalPath, sha256Hex, error). Idempotent: if
// the content-addressed file already exists the temp file is dropped and the
// existing path returned unchanged (zero-quota duplicate poll). Closes r.
//
// §107 anti-bluff: a writer that wrote zero bytes / a fixed path / re-wrote
// every duplicate would pass type checks. TestWriteContentAddressed... pins
// all three: exact path shape, byte-equality, and no .part residue.
//
// This is the channel-agnostic promotion of the tgram DownloadAttachment
// stream-hash-rename body (commons_messaging/channels/tgram/attachments.go,
// Wave 6) — same algorithm, generalized to any io.ReadCloser + channel.
func WriteContentAddressed(channel, mime string, r io.ReadCloser) (string, string, error) {
	defer r.Close()
	dir, err := InboxDir(channel)
	if err != nil { return "", "", err }
	tmp, err := os.CreateTemp(dir, "dl-*.part")
	if err != nil { return "", "", fmt.Errorf("channels.WriteContentAddressed: create temp: %w", err) }
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), r); err != nil { _ = tmp.Close(); cleanup(); return "", "", fmt.Errorf("channels.WriteContentAddressed: stream: %w", err) }
	if err := tmp.Close(); err != nil { cleanup(); return "", "", fmt.Errorf("channels.WriteContentAddressed: close temp: %w", err) }
	sumHex := hex.EncodeToString(hasher.Sum(nil))
	finalPath := filepath.Join(dir, sumHex+"."+MimeToExt(mime))
	if _, statErr := os.Stat(finalPath); statErr == nil { cleanup(); return finalPath, sumHex, nil } // idempotent: content == proof
	if err := os.Rename(tmpPath, finalPath); err != nil { cleanup(); return "", "", fmt.Errorf("channels.WriteContentAddressed: rename: %w", err) }
	return finalPath, sumHex, nil
}

// MimeToExt is the canonical mime→extension map. Promote the existing tgram-
// private mimeToExt switch (Wave 6 commons_messaging/channels/tgram/
// attachments.go:125-148) VERBATIM to this package-level func — copy the body
// 1:1: image/jpeg|jpg→jpg, image/png→png, image/gif→gif, image/webp→webp,
// video/mp4→mp4, audio/ogg|opus→ogg, audio/mpeg|mp3→mp3, application/pdf→pdf,
// text/plain→txt, default→bin.
func MimeToExt(mime string) string { /* switch mime { ... } — see above */ }
```

- [ ] **Step 5: Route tgram's DownloadAttachment through the helper**

In `commons_messaging/channels/tgram/attachments.go`, replace the home-dir + inbox + temp + hash + rename body of the package-level `DownloadAttachment` so it fetches the telebot reader and delegates to the shared helper. Replace the function body (keep the signature) with:
```go
func DownloadAttachment(ctx context.Context, bot *telebot.Bot, fileID, mime string) (string, string, error) {
	if bot == nil {
		return "", "", errors.New("tgram.DownloadAttachment: nil bot")
	}
	if fileID == "" {
		return "", "", errors.New("tgram.DownloadAttachment: empty fileID")
	}
	rc, err := bot.File(&telebot.File{FileID: fileID})
	if err != nil {
		return "", "", fmt.Errorf("tgram.DownloadAttachment: bot.File(%s): %w", fileID, err)
	}
	// channels.WriteContentAddressed closes rc.
	path, sum, werr := channels.WriteContentAddressed(string(commons.ChannelTelegram), mime, rc)
	if werr != nil {
		return "", "", fmt.Errorf("tgram.DownloadAttachment: %w", werr)
	}
	_ = ctx // telebot.v3 Bot.File is not ctx-aware; reserved.
	return path, sum, nil
}
```
Delete the now-unused tgram-private `mimeToExt` func (the package-level `channels.MimeToExt` replaces it). Update the tgram import block: add `"github.com/vasic-digital/herald/commons"` and `"github.com/vasic-digital/herald/commons_messaging/channels"`; remove `crypto/sha256`, `encoding/hex`, `io`, `os`, `path/filepath` if no longer referenced in the file (run `go build` to confirm which remain).

- [ ] **Step 6: Update the tgram attachment test for the new path shape**

In `commons_messaging/channels/tgram/attachments_test.go`, change every assertion that expects `.herald/inbox/<sha>.<ext>` to expect `.herald/inbox/tgram/<sha>.<ext>`. Find the path assertion (`filepath.Join(home, ".herald", "inbox", ...)`) and insert `"tgram"` before the sha component. If the test computes the expected path, add the `"tgram"` segment to that join.

- [ ] **Step 7: Run the tests to verify they pass**

Run:
```bash
go test ./commons_messaging/channels/ -run 'TestInboxDir|TestWriteContentAddressed' -count=1
go test -race -count=1 ./commons_messaging/channels/tgram/...
```
Expected: both PASS — the tgram suite proves the routing change is behaviour-preserving (modulo the new subdir).

- [ ] **Step 8: Quiescence + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass' commons_messaging/channels/ || echo clean`
```bash
git add commons_messaging/channels/inbox.go commons_messaging/channels/inbox_test.go \
        commons_messaging/channels/tgram/attachments.go \
        commons_messaging/channels/tgram/attachments_test.go docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T3 (HRD-112): per-channel inbox subdirs

channels.InboxDir(channel) + channels.WriteContentAddressed stream-hash-
rename helper land attachments under ~/.herald/inbox/<channel>/<sha>.<ext>.
mimeToExt promoted to package-level channels.MimeToExt. tgram routes its
DownloadAttachment through the shared helper; tgram attachment test updated
for the /tgram/ path segment. Idempotent + zero-buffer + no .part residue.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4 (HRD-113): Generalize the bot self-filter via `BotSelfIdentity`

**Goal:** Replace the tgram-private `shouldDropBotSelf(username)` with a channel-agnostic `channels.IsSelfEcho(ev, self)` that the inbound runtime applies uniformly. The adapter stamps the sender's native identity into `InboundEvent.Sender` so the filter compares the right field per `IdentityKind`.

**Files:**
- Create: `commons_messaging/channels/selffilter.go`
- Create: `commons_messaging/channels/selffilter_test.go`
- Modify: `commons_messaging/channels/tgram/subscribe.go` (stamp sender identity + delegate to `IsSelfEcho`)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-113 | task | in_progress | high | Wave 7 T4 — generalize bot self-filter via BotSelfIdentity (channel-agnostic IsSelfEcho) | 2026-05-27 | 2026-05-27 | spec V3 §32.9; Catalogue-Check: reuse (internal, 2026-05-27) |
```

- [ ] **Step 2: Write the failing test**

Create `commons_messaging/channels/selffilter_test.go` (imports: `testing`, `.../commons`, `.../commons_messaging/channels`). Helper `ev(isBot bool, kind, value)` builds an `InboundEvent` whose `Raw` is stamped via the same keys `StampSender` writes (use `channels.StampSender(raw, isBot, kind, value)` to build it — DRY against the production stamper):
```go
func ev(isBot bool, kind channels.IdentityKind, value string) commons.InboundEvent {
	raw := map[string]any{}
	channels.StampSender(raw, isBot, kind, value)
	return commons.InboundEvent{Sender: commons.Recipient{Channel: "x", ChannelUserID: "1"}, Raw: raw}
}
func TestIsSelfEchoUsername(t *testing.T) {
	self := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: "herald_bot"}
	if !channels.IsSelfEcho(ev(true, channels.IdentityUsername, "herald_bot"), self) { t.Fatal("bot-own username should be self-echo") }
	if channels.IsSelfEcho(ev(true, channels.IdentityUsername, "other_bot"), self) { t.Fatal("different bot must NOT be self-echo (multi-bot is real traffic)") }
	if channels.IsSelfEcho(ev(false, channels.IdentityUsername, "alice"), self) { t.Fatal("human sender must NOT be self-echo") }
}
func TestIsSelfEchoUserID(t *testing.T) {
	self := channels.SelfIdentity{Kind: channels.IdentityUserID, Value: "U0HERALD"}
	if !channels.IsSelfEcho(ev(true, channels.IdentityUserID, "U0HERALD"), self) { t.Fatal("Slack bot_user_id match should be self-echo") }
	if channels.IsSelfEcho(ev(true, channels.IdentityUserID, "U0OTHER"), self) { t.Fatal("different Slack bot id must NOT be self-echo") }
}
func TestIsSelfEchoEmptySelfNeverEchoes(t *testing.T) {
	// Empty self = echo-loop hazard; filter is conservative (KEEP) so misconfig
	// surfaces as duplicate traffic, not silent loss. Subscribe refuses to boot on empty self.
	if channels.IsSelfEcho(ev(true, channels.IdentityUsername, "herald_bot"), channels.SelfIdentity{}) { t.Fatal("empty self must never echo") }
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./commons_messaging/channels/ -run TestIsSelfEcho -count=1`
Expected: FAIL — undefined `channels.IsSelfEcho`.

- [ ] **Step 4: Write the generalized filter**

Create `commons_messaging/channels/selffilter.go`:
```go
package channels

import "github.com/vasic-digital/herald/commons"

// Raw-map keys the adapters stamp so the channel-agnostic filter can compare.
const (
	RawSenderIsBot       = "sender_is_bot"
	RawSenderIdentityKnd = "sender_identity_kind"
	RawSenderIdentity    = "sender_identity"
)

// StampSender records the sender's native identity into ev.Raw so IsSelfEcho
// can compare it against the bot's SelfIdentity without channel-specific
// knowledge. Adapters call this from their Subscribe handlers.
func StampSender(raw map[string]any, isBot bool, kind IdentityKind, value string) {
	if raw == nil {
		return
	}
	raw[RawSenderIsBot] = isBot
	raw[RawSenderIdentityKnd] = string(kind)
	raw[RawSenderIdentity] = value
}

// IsSelfEcho reports whether ev originates from THIS bot (self-echo) so the
// inbound runtime drops it before re-dispatching its own reply — the Wave 6
// §32.9 anti-echo-loop guarantee, now channel-agnostic.
//
// Scope is deliberately narrow: a DIFFERENT bot in the same conversation is
// KEPT (multi-bot collaboration is real subscriber traffic). An empty self
// Value never classifies as echo — Subscribe refuses to boot without a
// self-identity, so reaching here with empty self is a conservative KEEP that
// surfaces as duplicate traffic rather than silent message loss.
func IsSelfEcho(ev commons.InboundEvent, self SelfIdentity) bool {
	if self.Value == "" {
		return false
	}
	if ev.Raw == nil {
		return false
	}
	isBot, _ := ev.Raw[RawSenderIsBot].(bool)
	if !isBot {
		return false
	}
	kind, _ := ev.Raw[RawSenderIdentityKnd].(string)
	id, _ := ev.Raw[RawSenderIdentity].(string)
	return IdentityKind(kind) == self.Kind && id == self.Value
}
```

- [ ] **Step 5: Stamp sender identity in tgram Subscribe + delegate**

In `commons_messaging/channels/tgram/subscribe.go`:
1. Resolve `self := channels.SelfIdentity{Kind: channels.IdentityUsername, Value: selfUsername}` once after the `selfUsername` guard.
2. In each handler (`OnText`, `OnPhoto`, `OnDocument`, `OnVoice`), AFTER constructing `ev`, stamp the sender identity and replace the `shouldDropBotSelf(msg, selfUsername)` early-return with the generalized check applied to the built event. Concretely, in `OnText` replace:
```go
		if shouldDropBotSelf(msg, selfUsername) {
			return nil
		}
		ev := commons.InboundEvent{ ... }
```
with (stamp into `ev.Raw` then check):
```go
		ev := commons.InboundEvent{ ... } // unchanged construction
		senderBot, senderName := false, ""
		if msg.Sender != nil {
			senderBot = msg.Sender.IsBot
			senderName = msg.Sender.Username
		}
		channels.StampSender(ev.Raw, senderBot, channels.IdentityUsername, senderName)
		if channels.IsSelfEcho(ev, self) {
			return nil
		}
```
For `OnPhoto`/`OnDocument`/`OnVoice` (which call `buildEventWithAttachment`), stamp on the returned event before dispatch:
```go
		ev := buildEventWithAttachment(msg, msg.Caption, path, sumHex, mime, msg.Photo.FileSize)
		senderBot, senderName := false, ""
		if msg.Sender != nil {
			senderBot = msg.Sender.IsBot
			senderName = msg.Sender.Username
		}
		channels.StampSender(ev.Raw, senderBot, channels.IdentityUsername, senderName)
		if channels.IsSelfEcho(ev, self) {
			return nil
		}
		return h.Handle(ctx, ev)
```
3. Keep the existing `shouldDropBotSelf` func in place (the Wave 6 mutation gate M1 and `TestSubscribeBotSelfFilter` still target it) — it now becomes a thin helper the test exercises directly, while the live path uses `IsSelfEcho`. To avoid two code paths drifting, make `shouldDropBotSelf` delegate:
```go
func shouldDropBotSelf(msg *telebot.Message, selfUsername string) bool {
	if msg == nil || msg.Sender == nil {
		return false
	}
	raw := map[string]any{}
	channels.StampSender(raw, msg.Sender.IsBot, channels.IdentityUsername, msg.Sender.Username)
	return channels.IsSelfEcho(commons.InboundEvent{Raw: raw}, channels.SelfIdentity{Kind: channels.IdentityUsername, Value: selfUsername})
}
```
Add `"github.com/vasic-digital/herald/commons_messaging/channels"` to subscribe.go's imports.

- [ ] **Step 6: Run the tests to verify they pass**

Run:
```bash
go test ./commons_messaging/channels/ -run TestIsSelfEcho -count=1
go test -race -count=1 ./commons_messaging/channels/tgram/...
```
Expected: both PASS. The tgram `TestSubscribeBotSelfFilter` still passes via the delegating `shouldDropBotSelf`.

- [ ] **Step 7: Quiescence + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass' commons_messaging/channels/ || echo clean`
```bash
git add commons_messaging/channels/selffilter.go commons_messaging/channels/selffilter_test.go \
        commons_messaging/channels/tgram/subscribe.go docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T4 (HRD-113): generalize bot self-filter via BotSelfIdentity

channels.IsSelfEcho(ev, SelfIdentity) + channels.StampSender make the §32.9
anti-echo-loop filter channel-agnostic — compares the right native field per
IdentityKind (username/user_id/address). Empty self never echoes (Subscribe
refuses to boot without identity). tgram Subscribe stamps sender identity +
delegates; shouldDropBotSelf now delegates to IsSelfEcho so Wave 6 mutation
gate M1 + TestSubscribeBotSelfFilter stay load-bearing on the same code path.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5 (HRD-114): Multi-channel `pherald listen`

**Goal:** `HERALD_CHANNELS=tgram,slack` spins up one `Subscribe` goroutine per enabled channel, all feeding the SAME `inbound.Dispatcher`. Default (unset) = `tgram` only. Migrate the dispatcher's `TgramReplier` to a generic `Replier` so any channel can reply.

**Files:**
- Modify: `pherald/internal/inbound/dispatcher.go` (rename `TgramReplier` → `Replier`, generic signature)
- Modify: `pherald/cmd/pherald/listen.go` (multi-channel fan-in)
- Modify: `pherald/cmd/pherald/listen_test.go` (multi-channel hermetic test)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-114 | task | in_progress | high | Wave 7 T5 — multi-channel pherald listen (HERALD_CHANNELS fan-in: N Subscribe goroutines → 1 dispatcher) | 2026-05-27 | 2026-05-27 | spec V3 §32.2; Catalogue-Check: reuse (internal, 2026-05-27) |
```

- [ ] **Step 2: Write the failing multi-channel test**

Add to `pherald/cmd/pherald/listen_test.go` (two stub subscribers simulating tgram+slack; each publishes one synthetic event; assert BOTH reach the shared dispatcher AND both replies are recorded — §107: not "both goroutines started" but "both messages dispatched + both replies sent"):
```go
func TestRunListenMultiChannelFanIn(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	mkSub := func(channel string) func(context.Context, commons.InboundHandler) error {
		return func(ctx context.Context, h commons.InboundHandler) error {
			_ = h.Handle(ctx, commons.InboundEvent{EventID: channel + "-evt",
				Sender: commons.Recipient{Channel: channel, ChannelUserID: "100"},
				Body: commons.Body{Plain: "ping from " + channel}, Raw: map[string]any{"message_id": 7}})
			<-ctx.Done(); return ctx.Err()
		}
	}
	rec := &recordingReplier{onReply: func(text string) { mu.Lock(); seen[text]++; mu.Unlock() }}
	cfg := listenConfig{ProjectName: "Herald", Code: fakeCodeDispatcher{}, Replier: rec,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{"tgram": mkSub("tgram"), "slack": mkSub("slack")}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { time.Sleep(500 * time.Millisecond); cancel() }()
	if err := runListen(ctx, cfg); err != nil { t.Fatalf("runListen: %v", err) }
	mu.Lock(); defer mu.Unlock()
	if seen["ack (fake)"] < 2 { t.Fatalf("expected ≥2 replies (one per channel), got %v", seen) }
}
```
If `recordingReplier` lacks an `onReply func(string)` hook, add the field + invoke it in its `SendReply`. Add `sync`/`time` imports if missing.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pherald/cmd/pherald/ -run TestRunListenMultiChannelFanIn -count=1`
Expected: FAIL — `listenConfig` has no `Subscribers` field (only `Subscriber`); `runListen` only drives one subscriber.

- [ ] **Step 4: Rename `TgramReplier` → `Replier` (generic signature)**

In `pherald/internal/inbound/dispatcher.go`, rename the interface and widen it to the generic recipient/string signature:
```go
// Replier sends a reply that quotes the original message. Wave 7 generic
// signature: recipient + string ids let any channel (Telegram, Slack, …)
// satisfy it. The tgram adapter's SendReplyGeneric and the slack adapter's
// SendReply both match.
type Replier interface {
	SendReply(ctx context.Context, recipient commons.Recipient, body string, replyToID string, attachments []commons.Attachment) (string, error)
}
```
Update `Config.TgramReply` → `Config.Reply Replier`, `Dispatcher.reply Replier`, and `NewDispatcher`'s nil-check message. Update the two call sites in `Handle` + `fastPath` that call `d.reply.SendReply(ctx, chatID, ...)` — they currently pass `chatID int64` and `replyToID int`; change them to pass `commons.Recipient{Channel: ev.Sender.Channel, ChannelUserID: ev.Sender.ChannelUserID}` and `strconv.Itoa(replyToID)` (or `""` when 0). The return is now `(string, error)`; discard the string. Example for the `reply` action branch:
```go
	case "reply":
		replyToID, _ := extractReplyToMessageID(ev.Raw)
		rt := ""
		if replyToID > 0 {
			rt = strconv.Itoa(replyToID)
		}
		rcpt := commons.Recipient{Channel: ev.Sender.Channel, ChannelUserID: ev.Sender.ChannelUserID}
		if _, err := d.reply.SendReply(ctx, rcpt, reply.Text, rt, nil); err != nil {
			return fmt.Errorf("inbound: send reply: %w", err)
		}
		log.Printf("inbound dispatched: reply (event=%s channel=%s user=%s replyTo=%s)", ev.EventID, ev.Sender.Channel, ev.Sender.ChannelUserID, rt)
		return nil
```
Apply the same recipient/string conversion in `fastPath`'s success + error branches. Update `pherald/internal/inbound/dispatcher_test.go` + `reply_test.go` stub repliers to the new signature.

- [ ] **Step 5: Multi-channel fan-in in listen.go**

In `pherald/cmd/pherald/listen.go`:
1. Add `Subscribers map[string]func(ctx context.Context, h commons.InboundHandler) error` to `listenConfig` (keep the old single `Subscriber` field for back-compat in the existing single-channel test, OR migrate it — prefer migrating: replace `Subscriber` usages with a one-entry `Subscribers` map). Rename `Replier inbound.TgramReplier` → `Replier inbound.Replier`.
2. Add `loadEnabledChannels()` reading `HERALD_CHANNELS` (comma-split, trim, default `["tgram"]` when unset/empty).
3. In `loadListenConfigFromEnv`, for each enabled channel resolve its constructor via `channels.New(name, perChannelConfig(name))` where `perChannelConfig` reads the namespaced env (`HERALD_TGRAM_BOT_TOKEN`/`HERALD_TGRAM_CHAT_ID`, `HERALD_SLACK_BOT_TOKEN`/`HERALD_SLACK_APP_TOKEN`/`HERALD_SLACK_CHANNEL_ID`). Build the `Subscribers` map: `Subscribers[name] = ch.Subscribe`. Pick the FIRST enabled channel's adapter as the default `Replier` — but since replies route per-event by `ev.Sender.Channel`, store a `map[string]inbound.Replier` and wrap it in a routing `Replier` that dispatches by recipient channel:
```go
// channelRouter implements inbound.Replier by routing each reply to the
// adapter for recipient.Channel. A reply for an unregistered channel is a
// §107 fail-loud error (never silently dropped).
type channelRouter struct{ repliers map[string]inbound.Replier }

func (r *channelRouter) SendReply(ctx context.Context, rcpt commons.Recipient, body, replyToID string, atts []commons.Attachment) (string, error) {
	rep, ok := r.repliers[rcpt.Channel]
	if !ok {
		return "", fmt.Errorf("pherald listen: no replier for channel %q", rcpt.Channel)
	}
	return rep.SendReply(ctx, rcpt, body, replyToID, atts)
}
```
For tgram, the replier is a thin adapter over `*tgram.Adapter.SendReplyGeneric` (which already matches the generic signature). For slack (T6), `*slack.Adapter` satisfies the generic `SendReply` directly.
4. In `runListen`, replace the single `cfg.Subscriber(ctx, handler)` call with a goroutine-per-subscriber fan-in using `golang.org/x/sync/errgroup` (already an indirect dep) OR a manual `sync.WaitGroup` + error channel:
```go
	g, gctx := errgroup.WithContext(ctx)
	for name, sub := range cfg.Subscribers {
		name, sub := name, sub
		g.Go(func() error {
			if err := sub(gctx, handler); err != nil {
				if gctx.Err() != nil {
					return nil // clean cancel
				}
				return fmt.Errorf("pherald listen: channel %q subscribe: %w", name, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
```
Add `golang.org/x/sync/errgroup` + `github.com/vasic-digital/herald/commons_messaging/channels` + `.../channels/slack` (blank import for init registration) to listen.go imports, and add `golang.org/x/sync` to `pherald/go.mod` require if not already direct.

- [ ] **Step 6: Run the tests to verify they pass**

Run:
```bash
go test ./pherald/cmd/pherald/ -run 'TestRunListen' -count=1
go test -race -count=1 ./pherald/internal/inbound/...
```
Expected: PASS — multi-channel fan-in + the existing single-channel test (now driven through the one-entry map) both green.

- [ ] **Step 7: Quiescence + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass' pherald/ commons_messaging/ || echo clean`
```bash
git add pherald/internal/inbound/dispatcher.go pherald/internal/inbound/dispatcher_test.go \
        pherald/internal/inbound/reply_test.go pherald/cmd/pherald/listen.go \
        pherald/cmd/pherald/listen_test.go pherald/go.mod docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T5 (HRD-114): multi-channel pherald listen

HERALD_CHANNELS=tgram,slack spins up one Subscribe goroutine per enabled
channel (errgroup fan-in) feeding ONE inbound.Dispatcher. Default unset =
tgram only (Wave 6 preserved). inbound.TgramReplier → generic inbound.Replier
(recipient + string ids); channelRouter dispatches each reply to the adapter
for ev.Sender.Channel (fail-loud on unknown). Per-channel namespaced env.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6 (HRD-115): Slack channel adapter (Socket Mode)

**Goal:** A Slack adapter that satisfies `channels.Channel` — Socket Mode `Subscribe` (parity with Telegram long-poll), `OnMessage`/`OnFile` handlers, `Send`, `SendReply` (thread_ts), `BotSelfIdentity` (auth.test), `DownloadAttachment` (url_private_download). Proves the abstraction with a SECOND impl.

**Files:**
- Vendor: `submodules/slack-go` (git submodule of `github.com/slack-go/slack`)
- Create: `commons_messaging/channels/slack/slack.go`, `subscribe.go`, `send.go`, `attachments.go`, `selfidentity.go`
- Create: `commons_messaging/channels/slack/slack_test.go`, `send_test.go`, `attachments_test.go`
- Modify: `commons_messaging/go.mod` (add slack-go require + replace directive)

- [ ] **Step 1: Open the HRD + catalogue-check**

Add to `docs/Issues.md` (note the §11.4.74 catalogue-check in References):
```
| HRD-115 | task | in_progress | high | Wave 7 T6 — Slack channel adapter (Socket Mode): Send/SendReply(thread_ts)/Subscribe(OnMessage,OnFile)/BotSelfIdentity(auth.test)/DownloadAttachment | 2026-05-27 | 2026-05-27 | spec V3 §11 Slack rows + §32.2; Catalogue-Check: reuse slack-go/slack (vendored submodule, 2026-05-27) |
```

- [ ] **Step 2: Vendor the slack-go SDK as a submodule**

Run:
```bash
git submodule add https://github.com/slack-go/slack.git submodules/slack-go
git -C submodules/slack-go checkout v0.16.0   # pin a release tag; adjust to latest stable
```
Then in `commons_messaging/go.mod` add:
```
require github.com/slack-go/slack v0.16.0
replace github.com/slack-go/slack => ../submodules/slack-go
```
Run `go mod tidy` inside `commons_messaging` (or `go work sync`) and verify `go build ./commons_messaging/...` resolves the import. NOTE: slack-go's Socket Mode lives in `github.com/slack-go/slack/socketmode`; confirm the vendored tag includes it.

- [ ] **Step 3: Write the failing hermetic round-trip test**

Create `commons_messaging/channels/slack/send_test.go`. Use `httptest.NewServer` impersonating the Slack Web API (`r.ParseForm()` — slack-go posts `application/x-www-form-urlencoded`); the adapter's `BaseURL` points at it. Two functions assert real wire bytes:
```go
package slack_test
// imports: context, encoding/json, net/http, net/http/httptest, strings, testing,
//          .../commons, .../commons_messaging/channels/slack

func TestSlackSendCrossesWireWithText(t *testing.T) {
	var gotChannel, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") { http.Error(w, "bad path "+r.URL.Path, 404); return }
		_ = r.ParseForm(); gotChannel = r.FormValue("channel"); gotText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": gotChannel, "ts": "1654.0001"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C123", srv.URL)
	receipt, err := a.Send(context.Background(), commons.OutboundMessage{
		To: []commons.Recipient{{Channel: "slack", ChannelUserID: "C123"}}, Body: commons.Body{Plain: "hello slack"}})
	if err != nil { t.Fatalf("Send: %v", err) }
	if gotChannel != "C123" || gotText != "hello slack" { t.Fatalf("wire wrong: channel=%q text=%q", gotChannel, gotText) }
	if receipt.ChannelMsgID != "1654.0001" { t.Fatalf("ChannelMsgID=%q want ts (§107: real ts not synthetic)", receipt.ChannelMsgID) }
}

func TestSlackSendReplyUsesThreadTS(t *testing.T) {
	var gotThreadTS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm(); gotThreadTS = r.FormValue("thread_ts")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0002"})
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C123", srv.URL)
	id, err := a.SendReply(context.Background(), commons.Recipient{Channel: "slack", ChannelUserID: "C123"}, "threaded reply", "1654.0001", nil)
	if err != nil { t.Fatalf("SendReply: %v", err) }
	if gotThreadTS != "1654.0001" { t.Fatalf("thread_ts=%q want 1654.0001 (§107: reply must thread)", gotThreadTS) }
	if id != "1654.0002" { t.Fatalf("reply ts=%q want 1654.0002", id) }
}
```
Create `commons_messaging/channels/slack/slack_test.go` for interface-satisfaction + `BotSelfIdentity` (auth.test):
```go
package slack_test
// imports: context, encoding/json, net/http, net/http/httptest, strings, testing,
//          .../commons, .../commons_messaging/channels, .../commons_messaging/channels/slack

func TestSlackSatisfiesChannel(t *testing.T) {
	var c channels.Channel = slack.NewWithBaseURL("xoxb", "xapp", "C1", "http://localhost")
	if c.Name() != string(commons.ChannelSlack) { t.Fatalf("Name()=%q want slack", c.Name()) }
	if !c.Capabilities().Threads { t.Fatal("Slack Capabilities().Threads should be true") }
}

func TestSlackBotSelfIdentityViaAuthTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/auth.test") { http.Error(w, "bad "+r.URL.Path, 404); return }
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user_id": "U0HERALD", "user": "herald"})
	}))
	defer srv.Close()
	id, err := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL).BotSelfIdentity(context.Background())
	if err != nil { t.Fatalf("BotSelfIdentity: %v", err) }
	if id.Kind != channels.IdentityUserID || id.Value != "U0HERALD" { t.Fatalf("self=%+v want {user_id U0HERALD}", id) }
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./commons_messaging/channels/slack/ -count=1`
Expected: FAIL — package does not exist yet.

- [ ] **Step 5: Write the Slack adapter**

Create `commons_messaging/channels/slack/slack.go`:
```go
// Package slack is the Wave 7 Slack channel adapter — the SECOND concrete
// channels.Channel implementation, proving the abstraction Telegram defined.
//
// Transport: Socket Mode (WebSocket) for inbound parity with Telegram's
// long-poll (spec §32.2 Slack row). Outbound uses the Web API
// (chat.postMessage / chat.postMessage with thread_ts / files via
// files.info + url_private_download).
//
// §107 anti-bluff: every method crosses the wire — Send hits chat.postMessage
// and returns the real Slack `ts` (not a synthetic id); BotSelfIdentity hits
// auth.test; DownloadAttachment streams url_private_download into the per-
// channel inbox. The hermetic httptest suite counts each round-trip.
package slack

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// Adapter is the Slack channel adapter.
type Adapter struct {
	botToken  string // xoxb-...
	appToken  string // xapp-... (Socket Mode)
	channelID string // default outbound channel (C…)
	baseURL   string // httptest seam; "" => live Slack Web API
	api       *slack.Client
}

// New builds a live Slack adapter. botToken (xoxb-) is required; appToken
// (xapp-) is required only for Subscribe (Socket Mode). channelID is the
// default outbound destination.
func New(botToken, appToken, channelID string) *Adapter {
	return NewWithBaseURL(botToken, appToken, channelID, "")
}

// NewWithBaseURL is the httptest seam — baseURL overrides the Web API endpoint
// (slack-go appends method paths to APIURL, which MUST end with a slash) so
// send_test.go can assert wire bytes. Production uses New (baseURL "").
func NewWithBaseURL(botToken, appToken, channelID, baseURL string) *Adapter {
	opts := []slack.Option{}
	if baseURL != "" {
		u := baseURL
		if u[len(u)-1] != '/' { u += "/" }
		opts = append(opts, slack.OptionAPIURL(u))
	}
	return &Adapter{botToken: botToken, appToken: appToken, channelID: channelID, baseURL: baseURL, api: slack.New(botToken, opts...)}
}

// Name returns the canonical channel id.
func (a *Adapter) Name() string { return string(commons.ChannelSlack) }

// Capabilities per spec §11 Slack rows: mrkdwn, threads (thread_ts), file
// uploads, Block-Kit link/button. DeliveryCeiling Routed (postMessage ok).
func (a *Adapter) Capabilities() commons.Capabilities {
	return commons.Capabilities{Text: true, Markdown: true, HTML: false, Attachments: true,
		AttachmentMaxMiB: 1024, Threads: true, InteractiveURL: true, InteractiveCall: false,
		DeliveryCeiling: commons.DeliveryRouted}
}

// HealthCheck verifies the bot token via auth.test.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	_, err := a.api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack.HealthCheck: auth.test: %w", err)
	}
	return nil
}

// init registers the Slack adapter (Wave 7 T6). cfg.Token=xoxb-, cfg.AppToken=
// xapp-, cfg.Target=channel id, cfg.BaseURL=httptest seam.
func init() {
	channels.Register(string(commons.ChannelSlack), func(cfg channels.Config) (channels.Channel, error) {
		if cfg.Token == "" {
			return nil, fmt.Errorf("slack: cfg.Token (xoxb- bot token) required")
		}
		return NewWithBaseURL(cfg.Token, cfg.AppToken, cfg.Target, cfg.BaseURL), nil
	})
}
```
Create `commons_messaging/channels/slack/send.go` — `Send` (chat.postMessage, returns the real Slack `ts` in `Receipt.ChannelMsgID`; resolves channel from `msg.To[0].ChannelUserID` else `a.channelID`; prefers `Body.Markdown` else `Body.Plain`; empty body / empty ts → §107 error) and `SendReply` (signature `(ctx, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error)`):
```go
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	channelID := a.channelID
	if len(msg.To) > 0 && msg.To[0].ChannelUserID != "" {
		channelID = msg.To[0].ChannelUserID
	}
	text := msg.Body.Markdown
	if text == "" { text = msg.Body.Plain }
	if text == "" { return commons.Receipt{}, fmt.Errorf("slack.Send: Body has neither Markdown nor Plain") }
	start := time.Now()
	_, ts, err := a.api.PostMessageContext(ctx, channelID, slack.MsgOptionText(text, false))
	if err != nil { return commons.Receipt{}, fmt.Errorf("slack.Send: chat.postMessage to %s: %w", channelID, err) }
	if ts == "" { return commons.Receipt{}, fmt.Errorf("slack.Send: empty ts (§107 bluff guard)") }
	return commons.Receipt{Evidence: commons.DeliveryRouted, ChannelMsgID: ts, SentAt: time.Now(),
		LatencyMillis: time.Since(start).Milliseconds(), Native: map[string]any{"ts": ts, "channel": channelID}}, nil
}

func (a *Adapter) SendReply(ctx context.Context, recipient commons.Recipient, body, replyToID string, attachments []commons.Attachment) (string, error) {
	channelID := recipient.ChannelUserID
	if channelID == "" { channelID = a.channelID }
	if body == "" { return "", fmt.Errorf("slack.SendReply: empty body") }
	opts := []slack.MsgOption{slack.MsgOptionText(body, false)}
	if replyToID != "" { opts = append(opts, slack.MsgOptionTS(replyToID)) } // thread_ts — §107 reply-threading anchor
	_, ts, err := a.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil { return "", fmt.Errorf("slack.SendReply: chat.postMessage: %w", err) }
	if ts == "" { return "", fmt.Errorf("slack.SendReply: empty ts (§107 bluff guard)") }
	for i, att := range attachments { // each attachment threads under the same ts
		if att.Filename == "" { return "", fmt.Errorf("slack.SendReply: attachment[%d] empty Filename", i) }
		params := slack.UploadFileV2Parameters{Channel: channelID, File: att.Filename, Filename: filepath.Base(att.Filename), ThreadTimestamp: replyToID}
		if _, uerr := a.api.UploadFileV2Context(ctx, params); uerr != nil { return "", fmt.Errorf("slack.SendReply: upload attachment[%d]: %w", i, uerr) }
	}
	return ts, nil
}
```
Imports: `context`, `fmt`, `path/filepath`, `time`, `github.com/slack-go/slack`, `.../commons`.

Create `commons_messaging/channels/slack/selfidentity.go` — `BotSelfIdentity` via auth.test (empty user_id → echo-loop error):
```go
func (a *Adapter) BotSelfIdentity(ctx context.Context) (channels.SelfIdentity, error) {
	resp, err := a.api.AuthTestContext(ctx)
	if err != nil { return channels.SelfIdentity{}, fmt.Errorf("slack.BotSelfIdentity: auth.test: %w", err) }
	if resp.UserID == "" { return channels.SelfIdentity{}, fmt.Errorf("slack.BotSelfIdentity: empty user_id (echo-loop hazard)") }
	return channels.SelfIdentity{Kind: channels.IdentityUserID, Value: resp.UserID}, nil
}
```

Create `commons_messaging/channels/slack/attachments.go` — `DownloadAttachment` via files.info + GET url_private_download (Bearer token) → `channels.WriteContentAddressed("slack", mime, resp.Body)`:
```go
func (a *Adapter) DownloadAttachment(ctx context.Context, externalID, mime string) (string, string, error) {
	if externalID == "" { return "", "", fmt.Errorf("slack.DownloadAttachment: empty file id") }
	info, _, _, err := a.api.GetFileInfoContext(ctx, externalID, 0, 0)
	if err != nil { return "", "", fmt.Errorf("slack.DownloadAttachment: files.info(%s): %w", externalID, err) }
	if info.URLPrivateDownload == "" { return "", "", fmt.Errorf("slack.DownloadAttachment: file %s has no url_private_download", externalID) }
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.URLPrivateDownload, nil)
	req.Header.Set("Authorization", "Bearer "+a.botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", "", fmt.Errorf("slack.DownloadAttachment: GET: %w", err) }
	if resp.StatusCode != http.StatusOK { _ = resp.Body.Close(); return "", "", fmt.Errorf("slack.DownloadAttachment: status %d", resp.StatusCode) }
	effMime := mime
	if effMime == "" { effMime = info.Mimetype }
	return channels.WriteContentAddressed(string(commons.ChannelSlack), effMime, resp.Body) // closes resp.Body
}
```

Create `commons_messaging/channels/slack/subscribe.go`:
```go
package slack

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// Subscribe runs the Socket Mode event loop until ctx is cancelled,
// dispatching each inbound message + file-share event to h.Handle.
//
// §32.2 parity: Socket Mode is the continuous WebSocket transport (the Slack
// equivalent of Telegram's long-poll). appToken (xapp-) is required.
//
// §107 anti-bluff: a Subscribe that returns nil without invoking h would be a
// bluff. The bot self-filter (T4) is wired via BotSelfIdentity — an empty
// self-identity refuses to boot (echo-loop hazard), matching tgram.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	if a.appToken == "" {
		return fmt.Errorf("slack.Subscribe: app-level token (xapp-) required for Socket Mode")
	}
	self, err := a.BotSelfIdentity(ctx)
	if err != nil {
		return fmt.Errorf("slack.Subscribe: resolve self-identity: %w", err)
	}
	client := socketmode.New(slack.New(a.botToken, slack.OptionAppLevelToken(a.appToken)))
	go func() { _ = client.RunContext(ctx) }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt := <-client.Events:
			if evt.Type != socketmode.EventTypeEventsAPI { continue }
			eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok { continue }
			client.Ack(*evt.Request)
			inner, ok := eventsAPI.InnerEvent.Data.(*slackevents.MessageEvent)
			if !ok { continue }
			ev := commons.InboundEvent{
				Sender: commons.Recipient{Channel: string(commons.ChannelSlack), ChannelUserID: inner.Channel},
				Body:   commons.Body{Plain: inner.Text},
				Raw:    map[string]any{"message_id": inner.TimeStamp, "channel": inner.Channel, "thread_ts": inner.ThreadTimeStamp},
			}
			channels.StampSender(ev.Raw, inner.BotID != "", channels.IdentityUserID, inner.User)
			if channels.IsSelfEcho(ev, self) { continue }
			if inner.ThreadTimeStamp != "" { ev.Thread = &commons.ConversationRef{Channel: commons.ChannelSlack, ThreadID: inner.ThreadTimeStamp} }
			for _, f := range inner.Files { // file-share: download each into the inbox
				path, sum, derr := a.DownloadAttachment(ctx, f.ID, f.Mimetype)
				if derr != nil { return fmt.Errorf("slack.Subscribe: download file %s: %w", f.ID, derr) }
				ev.Attachments = append(ev.Attachments, commons.Attachment{Filename: path, MIMEType: f.Mimetype, SizeBytes: int64(f.Size), CID: sum})
			}
			if err := h.Handle(ctx, ev); err != nil { return fmt.Errorf("slack.Subscribe: handle: %w", err) }
		}
	}
}
```
NOTE: confirm the exact slack-go field names (`inner.Files`, `f.Mimetype`, `f.Size`, `socketmode.EventTypeEventsAPI`) against the vendored `v0.16.0` tag; adjust if the vendored version differs. The `slackevents.MessageEvent` Files field exists in recent versions; if absent in the pinned tag, gate file-download behind a capability comment and land it as a follow-up HRD (keep text inbound working).

- [ ] **Step 6: Run the Slack tests to verify they pass**

Run: `go test -race -count=1 ./commons_messaging/channels/slack/...`
Expected: PASS — Send wire bytes, thread_ts reply, auth.test self-identity, interface satisfaction all green. The Subscribe Socket Mode path is exercised by the T11 chaos test (hard to httptest a WebSocket cleanly); for T6 the unit tests cover Send/SendReply/BotSelfIdentity/DownloadAttachment over httptest.

- [ ] **Step 7: Commit the qa transcript for the hermetic round-trip**

Capture the httptest round-trip as the §107.x evidence:
```bash
mkdir -p docs/qa/HRD-115-hermetic
go test -race -count=1 -v ./commons_messaging/channels/slack/... > docs/qa/HRD-115-hermetic/slack_roundtrip.log 2>&1
```
The log records the wire-byte assertions (chat.postMessage channel/text, thread_ts, auth.test user_id) — the hermetic bidirectional transcript. A live Slack transcript SKIPs-with-reason until creds land (T9 E84).

- [ ] **Step 8: Quiescence + commit (NOTE: submodule add → push submodule origin too)**

Run: `grep -rEn 'MUTATED W7-|// always pass' commons_messaging/ || echo clean`
```bash
git add .gitmodules submodules/slack-go commons_messaging/go.mod commons_messaging/go.sum \
        commons_messaging/channels/slack/ docs/qa/HRD-115-hermetic/ docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T6 (HRD-115): Slack channel adapter (Socket Mode) — 2nd impl

slack.Adapter satisfies channels.Channel: Send (chat.postMessage, real ts),
SendReply (thread_ts + threaded file fan-out), Subscribe (Socket Mode
OnMessage + OnFile), BotSelfIdentity (auth.test user_id), DownloadAttachment
(files.info + url_private_download → per-channel inbox). Vendored slack-go as
submodule per §11.4.74 + spec vendored-SDK convention. Hermetic httptest
round-trip transcript under docs/qa/HRD-115-hermetic/.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```
WARNING (§104 inheritance gate): adding a submodule writes `.gitmodules`. The Herald inheritance gate I6 FORBIDS `<repo-root>/.gitmodules` ONLY when it re-introduces the `constitution/` submodule. Confirm I6's exact check before committing: run `bash tests/test_constitution_inheritance.sh` and verify it still passes with the slack-go entry. If I6 fails on ANY `.gitmodules`, the slack-go SDK must be vendored differently (e.g. a `go.mod replace` against a sibling clone) — surface this to the operator as a blocking ambiguity rather than weakening the gate.

---

### Task 7 (HRD-116): qaherald Slack `MessengerClient`

**Goal:** A Slack impl of qaherald's `MessengerClient` so the lifecycle test runs against Slack too — plus a channel-keyed builder the orchestrator uses to construct the right client.

**Files:**
- Create: `qaherald/internal/messenger/slack.go`
- Create: `qaherald/internal/messenger/slack_test.go`
- Create: `qaherald/internal/messenger/builder.go`
- Create: `qaherald/internal/messenger/builder_test.go`

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-116 | task | in_progress | middle | Wave 7 T7 — qaherald Slack MessengerClient + channel-keyed builder (lifecycle runs against Slack) | 2026-05-27 | 2026-05-27 | spec V3 §107.x; mirrors qaherald telegram.go; Catalogue-Check: reuse slack-go/slack@<sha> (2026-05-27) |
```

- [ ] **Step 2: Write the failing builder test**

Create `qaherald/internal/messenger/builder_test.go` (import `testing`, `.../qaherald/internal/messenger`):
```go
func TestBuilderTgram(t *testing.T) {
	c, err := messenger.Build(messenger.BuildConfig{Channel: "tgram", Token: "123:abc", ChatID: 456})
	if err != nil || c == nil { t.Fatalf("Build(tgram): c=%v err=%v", c, err) }
	_ = c.Close()
}
func TestBuilderSlack(t *testing.T) {
	c, err := messenger.Build(messenger.BuildConfig{Channel: "slack", Token: "xoxb-x", ChannelID: "C1", BaseURL: "http://localhost"})
	if err != nil || c == nil { t.Fatalf("Build(slack): c=%v err=%v", c, err) }
	_ = c.Close()
}
func TestBuilderUnknownErrors(t *testing.T) {
	if _, err := messenger.Build(messenger.BuildConfig{Channel: "nope"}); err == nil { t.Fatal("Build(unknown) should error") }
}
```

- [ ] **Step 3: Write the failing Slack client test**

Create `qaherald/internal/messenger/slack_test.go` (mirror telegram_test.go's httptest pattern; imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `strings`, `testing`, `.../qaherald/internal/messenger`):
```go
func TestSlackClientSendCrossesWire(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") { http.Error(w, "bad "+r.URL.Path, 404); return }
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0001"})
	}))
	defer srv.Close()
	id, err := messenger.NewSlackClient("xoxb-test", "C1", srv.URL).Send(context.Background(), "hello from qa")
	if err != nil { t.Fatalf("Send: %v", err) }
	if hits != 1 { t.Fatalf("expected 1 chat.postMessage hit, got %d (§107: no-op Send is a bluff)", hits) }
	if id != messenger.MessageID("1654.0001") { t.Fatalf("MessageID=%q want ts", id) }
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./qaherald/internal/messenger/ -run 'TestBuilder|TestSlackClient' -count=1`
Expected: FAIL — undefined `messenger.Build`, `messenger.BuildConfig`, `messenger.NewSlackClient`.

- [ ] **Step 5: Write the Slack client + builder**

Create `qaherald/internal/messenger/slack.go` mirroring `telegram.go`'s raw-HTTP design (small client against the Slack Web API; httptest seam via baseURL; token never in error messages). It MUST satisfy the `MessengerClient` interface: `Me` (auth.test → user/user_id), `Send` (chat.postMessage), `SendPhoto`/`SendDocument`/`SendVoice` (files.upload V2 multipart), `WaitForReply`/`GetUpdates` (Slack does not have getUpdates — qaherald's Slack inbound uses `conversations.history` polling with a cursor/oldest-ts watermark; document this divergence in the file header), `Download` (url_private_download), `Preflight` (auth.test + conversations.info → ChatType), `Close`. Key skeleton:
```go
// Package messenger — Slack impl of MessengerClient (Wave 7 T7).
//
// Design parity with telegram.go: a small raw HTTP client against the Slack
// Web API (NOT slack-go) so offset/watermark bookkeeping for WaitForReply is
// fully controlled and the httptest server in slack_test.go counts every
// round-trip. Slack has no getUpdates; GetUpdates is implemented as
// conversations.history with an `oldest` ts watermark — non-matching messages
// stay visible to subsequent polls (the lifecycle WaitForReply contract).
//
// Security: the xoxb token NEVER appears in error messages.
package messenger

type SlackClient struct {
	token     string
	channelID string
	baseURL   string
	httpc     *http.Client
	mu        sync.Mutex
	meUser    string
	meID      string
}

func NewSlackClient(token, channelID, baseURL string) *SlackClient { /* ... */ }

func (c *SlackClient) Me(ctx context.Context) (string, int64, error)        { /* auth.test */ }
func (c *SlackClient) Send(ctx context.Context, text string) (MessageID, error) { /* chat.postMessage; return ts */ }
// SendPhoto/SendDocument/SendVoice → files.upload V2 multipart.
// GetUpdates → conversations.history with oldest=offset-derived ts.
// WaitForReply → poll GetUpdates until pred matches or ctx deadline.
// Download → GET url_private_download with Bearer token.
// Preflight → auth.test + conversations.info → PreflightReport.
// Close → no-op (idempotent).
```
Implement each method fully (no TODOs) — the file mirrors telegram.go's already-shipped patterns; reuse its multipart upload + JSON decode helpers (extract shared helpers into a `messenger/httpjson.go` if it reduces duplication, but do not over-DRY). MessageID for Slack is the dotted-float ts string.

Create `qaherald/internal/messenger/builder.go`:
```go
package messenger

import "fmt"

// BuildConfig carries channel-agnostic construction inputs for Build.
type BuildConfig struct {
	Channel   string // "tgram" | "slack"
	Token     string // bot token (Telegram token / Slack xoxb-)
	ChatID    int64  // Telegram numeric chat id
	ChannelID string // Slack channel id (C…)
	BaseURL   string // httptest seam; "" => live endpoint
}

// Build constructs the MessengerClient for cfg.Channel. Unknown channels
// error (no silent nil) — a qaherald run against an unsupported channel must
// fail loud, not no-op.
func Build(cfg BuildConfig) (MessengerClient, error) {
	switch cfg.Channel {
	case "tgram", "":
		return NewTelegramClient(cfg.Token, cfg.ChatID, cfg.BaseURL), nil
	case "slack":
		return NewSlackClient(cfg.Token, cfg.ChannelID, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("messenger.Build: unknown channel %q", cfg.Channel)
	}
}
```
Confirm the existing Telegram client constructor name (`NewTelegramClient` or similar) in telegram.go and match it; adjust the `tgram` case accordingly.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -race -count=1 ./qaherald/internal/messenger/...`
Expected: PASS — including the existing Telegram tests (the builder + Slack additions must not regress them).

- [ ] **Step 7: Quiescence + commit**

Run: `grep -rEn 'MUTATED W7-|// always pass' qaherald/ || echo clean`
```bash
git add qaherald/internal/messenger/slack.go qaherald/internal/messenger/slack_test.go \
        qaherald/internal/messenger/builder.go qaherald/internal/messenger/builder_test.go \
        docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T7 (HRD-116): qaherald Slack MessengerClient + channel-keyed builder

messenger.NewSlackClient satisfies MessengerClient via the Slack Web API
(raw HTTP, parity with telegram.go): Me/auth.test, Send/chat.postMessage,
file uploads, GetUpdates via conversations.history watermark, WaitForReply,
Download/url_private_download, Preflight. messenger.Build(channel) constructs
the right client (unknown → fail-loud). Token never in error text.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8 (HRD-117): Spec V3 §11.0 + §32.2 update + §43 row

**Goal:** Update the spec to mark Telegram + Slack as live and document the `channels.Channel` interface. Per the spec-change rule, this triggers the implied doc ripple — but all implied code already landed in T1-T7, so this task is the spec catch-up plus a §43 command-catalogue row for multi-channel `pherald listen`.

**Files:**
- Modify: `docs/specs/mvp/specification.V3.md` (§11.0 note, §32.2 status column, §43 row)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-117 | task | in_progress | low | Wave 7 T8 — spec V3 §11.0 channels.Channel note + §32.2 Telegram/Slack live + §43 multi-channel listen row | 2026-05-27 | 2026-05-27 | spec V3 §11.0/§32.2/§43; Catalogue-Check: n/a (doc, 2026-05-27) |
```

- [ ] **Step 2: Add the channels.Channel note to §11.0**

In `docs/specs/mvp/specification.V3.md`, after the `Channel` interface code block in §11.0 (around line 1197), add a prose note:
```
> **Wave 7 inbound-runtime extension.** The §11.0 `commons.Channel` above is the
> outbound contract every flavor's `serve` path consumes. The inbound runtime
> (`pherald listen`, §32) needs three additional per-adapter methods — quoted
> `SendReply`, `BotSelfIdentity` (anti-echo self-identity, §32.9), and
> content-addressed `DownloadAttachment`. These live on the richer
> `commons_messaging/channels.Channel` interface, which embeds `commons.Channel`
> and adds only the inbound methods. Adapters register with the
> `commons_messaging/channels` name→constructor registry via `init()`; the
> generalized self-filter `channels.IsSelfEcho` compares each adapter's native
> identity (Telegram username / Slack bot_user_id / Email From) uniformly.
> Attachments land under `~/.herald/inbox/<channel>/<sha256>.<ext>`.
```

- [ ] **Step 3: Mark Telegram + Slack live in the §32.2 cadence table**

In the §32.2 table (around line 3300), add a Status column or annotate the Telegram + Slack rows. Change:
```
| Telegram | Bot API `getUpdates` long-poll (timeout 25 s) | continuous, but a 30 s safety-net timer also fires `getUpdates` if the long-poll thread stalled |
| Slack | Socket Mode (WebSocket) OR Events API webhook | continuous on Socket Mode; 30 s timer as keepalive ping |
```
to append ` — **LIVE (Wave 6 / Wave 7)**` to the Telegram mechanism cell and ` — **LIVE (Wave 7 T6 — Socket Mode)**` to the Slack mechanism cell. Leave Max/Discord/Email/etc. unannotated (still planned).

- [ ] **Step 4: Add the §43 command-catalogue row**

Find the §43 command catalogue table and add a row documenting the multi-channel surface (match the existing row format exactly):
```
| `pherald listen` (multi-channel) | `HERALD_CHANNELS=tgram,slack` runs one Subscribe goroutine per enabled channel into one inbound.Dispatcher; per-channel namespaced env; default `tgram`. | Wave 7 T5 | live |
```

- [ ] **Step 5: Regenerate spec siblings (if tracked) + commit**

Per CLAUDE.md, spec edits may have PDF/HTML siblings. Check: `ls docs/specs/mvp/specification.V3.* 2>/dev/null`. If siblings exist, run `bash scripts/export_docs.sh docs/specs/mvp/specification.V3.md`; otherwise skip.
```bash
git add docs/specs/mvp/specification.V3.md docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T8 (HRD-117): spec V3 §11.0 channels.Channel + §32.2 live + §43 row

§11.0 gains the Wave 7 inbound-runtime extension note (channels.Channel
embeds commons.Channel + SendReply/BotSelfIdentity/DownloadAttachment +
registry + IsSelfEcho + per-channel inbox). §32.2 marks Telegram + Slack
LIVE. §43 documents multi-channel pherald listen (HERALD_CHANNELS).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9 (HRD-118): e2e_bluff_hunt multi-channel invariants

**Goal:** Add E81..E88 to `scripts/e2e_bluff_hunt.sh` covering the channel registry, multi-channel resolution, the Slack hermetic round-trip, and a live Slack round-trip (SKIP-with-reason when creds absent).

**Files:**
- Modify: `scripts/e2e_bluff_hunt.sh`

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-118 | task | in_progress | high | Wave 7 T9 — e2e_bluff_hunt E81-E88 multi-channel invariants (registry resolves tgram+slack; Slack round-trip live-or-SKIP) | 2026-05-27 | 2026-05-27 | spec V3 §11.4; Catalogue-Check: n/a (test, 2026-05-27) |
```

- [ ] **Step 2: Add the E81-E88 invariants**

Append to `scripts/e2e_bluff_hunt.sh` (after the E80 block, using the existing `check`/`check_skip` helpers — read the file's helper signatures first to match exactly). The invariants:
```bash
# Wave 7 multi-channel framework (added 2026-05-27):
#   E81. channels registry resolves BOTH tgram + slack by name (Go test that
#        asserts channels.Names() contains both).
#   E82. tgram still satisfies channels.Channel after the refactor (T1 test).
#   E83. slack adapter hermetic round-trip — Send wire-bytes + thread_ts +
#        auth.test self-identity (T6 httptest suite green).
#   E84. LIVE Slack round-trip — qaherald Slack MessengerClient Send + reply
#        against the real workspace; SKIP-with-reason when HERALD_SLACK_BOT_TOKEN
#        absent.
#   E85. per-channel inbox isolation — channels.WriteContentAddressed lands
#        under inbox/<channel>/ (T3 test).
#   E86. generalized self-filter drops bot-own across identity kinds (T4 test).
#   E87. multi-channel pherald listen fan-in — two stub channels both dispatch
#        + both reply (T5 test).
#   E88. qaherald messenger.Build resolves tgram + slack (T7 test).

check "E81 channels registry resolves tgram+slack" \
    "go test -run 'TestRegistry|TestTgramRegisteredViaInit' -count=1 ./commons_messaging/channels/"
check "E82 tgram satisfies channels.Channel (pure-refactor)" \
    "go test -run TestTgramSatisfiesChannel -count=1 ./commons_messaging/channels/"
check "E83 slack hermetic round-trip (Send/thread_ts/auth.test)" \
    "go test -race -count=1 ./commons_messaging/channels/slack/..."
if [ -n "${HERALD_SLACK_BOT_TOKEN:-}" ] && [ -n "${HERALD_SLACK_CHANNEL_ID:-}" ]; then
    check "E84 LIVE Slack round-trip" \
        "go test -tags=live -run TestSlackLiveRoundTrip -count=1 ./qaherald/internal/messenger/..."
else
    check_skip "E84 LIVE Slack round-trip" \
        "SKIP: HERALD_SLACK_BOT_TOKEN / HERALD_SLACK_CHANNEL_ID absent — live Slack creds required (§107.x evidence pending operator run)"
fi
check "E85 per-channel inbox isolation" \
    "go test -run TestInboxDirIsPerChannel -count=1 ./commons_messaging/channels/"
check "E86 generalized self-filter across identity kinds" \
    "go test -run TestIsSelfEcho -count=1 ./commons_messaging/channels/"
check "E87 multi-channel pherald listen fan-in" \
    "go test -run TestRunListenMultiChannelFanIn -count=1 ./pherald/cmd/pherald/"
check "E88 qaherald messenger.Build resolves tgram+slack" \
    "go test -run TestBuilder -count=1 ./qaherald/internal/messenger/"
```
Confirm the exact name of the SKIP helper in the script (it may be `check_skip`, `skip`, or inline). If no skip helper exists, add one mirroring the existing `check` (prints `SKIP <name>` + bumps a skip counter, does NOT bump fail). Update the header comment's invariant count (`Eighty invariants` → `Eighty-eight invariants`) and the E-range description.

- [ ] **Step 3: Run the e2e suite (hermetic subset)**

Run: `bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -40`
Expected: E81-E83, E85-E88 PASS; E84 SKIP-with-reason (no live creds). NO new FAILs. If any pre-existing invariant regressed, fix at root cause before continuing.

- [ ] **Step 4: Commit**

Run: `grep -rEn 'MUTATED W7-' scripts/ || echo clean`
```bash
git add scripts/e2e_bluff_hunt.sh docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T9 (HRD-118): e2e_bluff_hunt E81-E88 multi-channel invariants

Registry resolves tgram+slack (E81), tgram pure-refactor satisfies the
interface (E82), Slack hermetic round-trip (E83), LIVE Slack SKIP-with-reason
(E84), per-channel inbox isolation (E85), generalized self-filter (E86),
multi-channel listen fan-in (E87), qaherald Build (E88).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 10 (HRD-119): Wave 7 paired §1.1 mutation gate

**Goal:** A paired mutation gate proving the registry resolution + generalized self-filter detectors are load-bearing — mutate registry to drop a channel → assert resolution FAILs; mutate self-filter → assert echo-loop detection FAILs.

**Files:**
- Create: `tests/test_wave7_mutation_meta.sh`

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-119 | task | in_progress | high | Wave 7 T10 — paired §1.1 mutation gate (drop registry entry → resolution FAIL; blank self-filter → echo-loop FAIL) | 2026-05-27 | 2026-05-27 | spec V3 §11.4 + §1.1 + §107.y; Catalogue-Check: n/a (test, 2026-05-27) |
```

- [ ] **Step 2: Write the mutation gate**

Create `tests/test_wave7_mutation_meta.sh` modelled on `tests/test_wave6_mutation_meta.sh` (reuse its `run_paired`, `assert_restored`, `check_quiescence`, `.git/MUTATION_IN_PROGRESS` lockfile, trap-on-exit cleanup, and pre-flight dirty-tree guard verbatim — they are the canonical §107.y prototype). Three mutations:
```bash
#!/usr/bin/env bash
# tests/test_wave7_mutation_meta.sh — Paired §1.1 mutation test for Wave 7
# (generic messenger-channel framework).
#
# Three mutations:
#
#   M1. Blank IsSelfEcho in commons_messaging/channels/selffilter.go:
#       body → `return false` (every bot-own message kept → echo-loop hazard).
#       Detector: TestIsSelfEchoUsername sub-assertion (bot-own IS echo).
#
#   M2. Drop the slack registry entry in
#       commons_messaging/channels/slack/slack.go: comment out the
#       channels.Register call in init(). Detector: TestTgramRegisteredViaInit
#       is tgram-only; use a dedicated detector TestSlackRegisteredViaInit
#       (add it to slack_test.go in this task) that asserts channels.Names()
#       contains "slack"; with the mutation it does not → FAIL.
#
#   M3. Drop thread_ts in commons_messaging/channels/slack/send.go:
#       remove the `opts = append(opts, slack.MsgOptionTS(replyToID))` line.
#       Detector: TestSlackSendReplyUsesThreadTS asserts thread_ts=="1654.0001"
#       on the wire; with the mutation the field is absent → FAIL.
#
# Returns 0 only when every mutation causes its detector to FAIL AND every
# detector returns to PASS after restore AND no MUTATED W7-M* markers leaked.
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

SELFFILTER_GO="${REPO_ROOT}/commons_messaging/channels/selffilter.go"
SLACK_GO="${REPO_ROOT}/commons_messaging/channels/slack/slack.go"
SLACK_SEND_GO="${REPO_ROOT}/commons_messaging/channels/slack/send.go"
# ... (copy the pre-flight dirty-tree guard, lockfile, trap, run_paired,
#      assert_restored, check_quiescence helpers from test_wave6 verbatim,
#      retargeting the file list to the three Wave 7 targets) ...
```
Add the `TestSlackRegisteredViaInit` detector to `commons_messaging/channels/slack/slack_test.go`:
```go
func TestSlackRegisteredViaInit(t *testing.T) {
	for _, n := range channels.Names() {
		if n == string(commons.ChannelSlack) {
			return
		}
	}
	t.Fatalf("slack not registered via init(); Names()=%v", channels.Names())
}
```
Write the three `run_paired` invocations with perl one-liners that inject the `MUTATED W7-M<n>` anchor (mirror the Wave 6 perl-substitution style — match the exact source lines). End with the quiescence assertion + tally block (copy from Wave 6).

- [ ] **Step 3: Run the mutation gate**

Run: `bash tests/test_wave7_mutation_meta.sh`
Expected: `Result: 4 PASS / 0 FAIL` (3 mutations + 1 quiescence) → `PASS: wave7 mutation gate (3 paired)`. Each mutation's detector FAILs on the mutated build and PASSes after restore; no marker leaks.

- [ ] **Step 4: Confirm the tree is clean post-gate (§107.y)**

Run: `git status --porcelain && grep -rn 'MUTATED W7-' commons_messaging/ tests/ || echo clean`
Expected: only `tests/test_wave7_mutation_meta.sh` (untracked) + the `TestSlackRegisteredViaInit` addition staged; NO `MUTATED W7-` markers in production source.

- [ ] **Step 5: Commit**

```bash
git add tests/test_wave7_mutation_meta.sh commons_messaging/channels/slack/slack_test.go \
        docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T10 (HRD-119): paired §1.1 mutation gate (3 mutations)

M1 blank IsSelfEcho → echo-loop detector FAILs; M2 drop slack registry
init() → TestSlackRegisteredViaInit FAILs; M3 drop slack thread_ts → reply-
threading detector FAILs. Each restores byte-for-byte + post-restore PASS;
§107.y quiescence asserted. Reuses the Wave 6 canonical lockfile + trap.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 11 (HRD-120): §11.4.85 stress + chaos for the multi-channel runtime

**Goal:** Mandate-required (§11.4.85). Stress: concurrent inbound from 2 channels under sustained load. Chaos: kill one channel's Subscribe goroutine mid-run → assert the OTHER keeps running + the dispatcher stays healthy (no shared-state corruption, no deadlock).

**Files:**
- Create: `pherald/cmd/pherald/stress_chaos_test.go`
- Create: `docs/qa/HRD-120/stress_chaos/` (captured evidence)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-120 | task | in_progress | high | Wave 7 T11 — §11.4.85 stress+chaos for multi-channel runtime (concurrent 2-channel inbound + kill-one-Subscribe fault injection) | 2026-05-27 | 2026-05-27 | spec V3 §11.4.85; Catalogue-Check: n/a (test, 2026-05-27) |
```

- [ ] **Step 2: Write the failing stress + chaos test**

Create `pherald/cmd/pherald/stress_chaos_test.go` (package `main`; imports `context`, `fmt`, `sync/atomic`, `testing`, `time`, `.../commons`):
```go
// TestMultiChannelStress: 2 stub channels x 200 concurrent inbound through one
// shared dispatcher; assert EVERY message dispatched (no loss/deadlock/race —
// run with -race). §11.4.85 stress layer.
func TestMultiChannelStress(t *testing.T) {
	const perChannel = 200
	var dispatched int64
	rec := &recordingReplier{onReply: func(string) { atomic.AddInt64(&dispatched, 1) }}
	mkBurst := func(channel string) func(context.Context, commons.InboundHandler) error {
		return func(ctx context.Context, h commons.InboundHandler) error {
			for i := 0; i < perChannel; i++ {
				_ = h.Handle(ctx, commons.InboundEvent{EventID: fmt.Sprintf("%s-%d", channel, i),
					Sender: commons.Recipient{Channel: channel, ChannelUserID: "1"},
					Body: commons.Body{Plain: "load"}, Raw: map[string]any{"message_id": i}})
			}
			<-ctx.Done(); return ctx.Err()
		}
	}
	cfg := listenConfig{ProjectName: "Herald", Code: fakeCodeDispatcher{}, Replier: rec,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{"tgram": mkBurst("tgram"), "slack": mkBurst("slack")}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { time.Sleep(2 * time.Second); cancel() }()
	_ = runListen(ctx, cfg)
	if got := atomic.LoadInt64(&dispatched); got != int64(2*perChannel) {
		t.Fatalf("stress: dispatched %d want %d (message loss under load)", got, 2*perChannel)
	}
}

// TestMultiChannelChaosKillOneSubscribe: inject a fault — one channel's
// Subscribe returns an error mid-run — and assert the OTHER kept dispatching +
// runListen propagates the fault (no deadlock/hang). §11.4.85 chaos layer.
func TestMultiChannelChaosKillOneSubscribe(t *testing.T) {
	var survivorN int64
	rec := &recordingReplier{onReply: func(string) { atomic.AddInt64(&survivorN, 1) }}
	dying := func(ctx context.Context, h commons.InboundHandler) error { return fmt.Errorf("injected fault: tgram Subscribe died") }
	survivor := func(ctx context.Context, h commons.InboundHandler) error {
		tk := time.NewTicker(20 * time.Millisecond); defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tk.C:
				_ = h.Handle(ctx, commons.InboundEvent{EventID: "slack-evt",
					Sender: commons.Recipient{Channel: "slack", ChannelUserID: "1"},
					Body: commons.Body{Plain: "alive"}, Raw: map[string]any{"message_id": 1}})
			}
		}
	}
	cfg := listenConfig{ProjectName: "Herald", Code: fakeCodeDispatcher{}, Replier: rec,
		Subscribers: map[string]func(context.Context, commons.InboundHandler) error{"tgram": dying, "slack": survivor}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runListen(ctx, cfg) }()
	select {
	case err := <-done:
		if err == nil { t.Fatal("chaos: runListen returned nil; expected injected fault to propagate") }
	case <-time.After(4 * time.Second):
		t.Fatal("chaos: runListen hung after one Subscribe died (deadlock)")
	}
	if atomic.LoadInt64(&survivorN) == 0 { t.Fatal("chaos: survivor never dispatched — fault took down whole runtime") }
}
```
NOTE: the chaos test pins a design property — `errgroup.WithContext` cancels the group when the dying subscriber returns an error, so the survivor's `ctx.Done()` fires and runListen returns the fault. This is the intended behaviour: a dead channel surfaces fail-loud rather than silently degrading. If the operator wants per-channel resilience (one channel dies, others keep serving indefinitely), that is a DIFFERENT design (supervised restart) — note it as a Wave 8 follow-up HRD in the test comment, do NOT silently implement it here.

- [ ] **Step 3: Run the stress + chaos test (with -race)**

Run: `go test -race -count=1 -run 'TestMultiChannelStress|TestMultiChannelChaos' ./pherald/cmd/pherald/`
Expected: PASS, no race detector hits. If stress shows message loss, the fan-in has a shared-state bug — fix at root cause (the dispatcher must be concurrency-safe across N subscriber goroutines; the `inbound.Dispatcher` holds only immutable config after construction, so the likely culprit is a non-thread-safe `Replier` or journal — guard it).

- [ ] **Step 4: Capture the evidence artefact (§11.4.85 + §107.x)**

```bash
mkdir -p docs/qa/HRD-120/stress_chaos
go test -race -count=1 -v -run 'TestMultiChannelStress|TestMultiChannelChaos' ./pherald/cmd/pherald/ \
    > docs/qa/HRD-120/stress_chaos/run.log 2>&1
```
The log is the captured-evidence anchor the matching e2e invariant cites.

- [ ] **Step 5: Commit**

Run: `grep -rEn 'MUTATED W7-' pherald/ || echo clean`
```bash
git add pherald/cmd/pherald/stress_chaos_test.go docs/qa/HRD-120/ docs/Issues.md docs/Fixed.md
git commit -m "$(cat <<'EOF'
Wave 7 T11 (HRD-120): §11.4.85 stress+chaos for multi-channel runtime

Stress: 2 channels x 200 concurrent inbound through one dispatcher, -race,
zero message loss. Chaos: kill one Subscribe goroutine mid-run → survivor
keeps dispatching + runListen propagates the fault (no deadlock/hang).
Captured evidence under docs/qa/HRD-120/stress_chaos/run.log.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

### Task 12 (HRD-121): Docs + Issues→Fixed + v0.6.0 + 4-mirror push

**Goal:** Per-channel operator guide, atomic Issues→Fixed migration for all Wave 7 HRDs, CLAUDE.md/Status.md/CONTINUATION.md refresh, tag v0.6.0 (operator-gated on live evidence), and detached 4-mirror push.

**Files:**
- Create: `docs/guides/MESSENGER_CHANNELS.md`
- Modify: `docs/Issues.md`, `docs/Fixed.md`, `docs/Status.md`, `docs/CONTINUATION.md`, `CLAUDE.md`
- Modify: `quickstart/.env.example` (per-channel env reservations)

- [ ] **Step 1: Open the HRD**

Add to `docs/Issues.md`:
```
| HRD-121 | task | in_progress | middle | Wave 7 T12 — docs/guides/MESSENGER_CHANNELS.md + Issues→Fixed + v0.6.0 + 4-mirror push | 2026-05-27 | 2026-05-27 | spec V3 §107.x; Catalogue-Check: n/a (doc/release, 2026-05-27) |
```

- [ ] **Step 2: Write the per-channel operator guide**

Create `docs/guides/MESSENGER_CHANNELS.md` with the centered logo header (relative path `../../assets/logo/herald_logo_square_128.png` for `docs/guides/` depth) followed by the metadata table (Revision/Created/Last modified/Status). Sections:
- **The channels.Channel interface** — the six-method contract + why it embeds commons.Channel.
- **The registry** — how `init()` registration works; `HERALD_CHANNELS` resolution.
- **Per-channel inbox** — `~/.herald/inbox/<channel>/<sha256>.<ext>`.
- **Telegram** — env vars (`HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`), bot setup, privacy-mode-off requirement, getMe self-identity.
- **Slack** — env vars (`HERALD_SLACK_BOT_TOKEN` xoxb-, `HERALD_SLACK_APP_TOKEN` xapp- for Socket Mode, `HERALD_SLACK_CHANNEL_ID`), app manifest scopes (`chat:write`, `files:read`, `files:write`, `channels:history`, Socket Mode + `connections:write`), auth.test self-identity, thread_ts replies.
- **Multi-channel `pherald listen`** — `HERALD_CHANNELS=tgram,slack`, one Subscribe goroutine per channel, fail-loud routing.
- **Adding a new channel (Max/Email/Discord)** — the implementer checklist: satisfy `channels.Channel`, register via `init()`, add namespaced env, add qaherald `MessengerClient` + builder case, add e2e + mutation detectors.
- **§107.x QA evidence** — where transcripts land per channel.

Run the logo injector: `python3 scripts/branding_inject_logo.py docs/guides/MESSENGER_CHANNELS.md`. Do NOT export PDF/HTML siblings unless the operator asks (per CLAUDE.md); note in the commit that siblings are pending operator request.

- [ ] **Step 3: Reserve per-channel env in .env.example**

In `quickstart/.env.example`, add the Slack block + Max/Email reservations under the existing Telegram block:
```
# --- Slack (Wave 7 T6 — LIVE) ---
# HERALD_SLACK_BOT_TOKEN=xoxb-...        # Bot User OAuth Token (chat:write, files:read/write, channels:history)
# HERALD_SLACK_APP_TOKEN=xapp-...        # App-Level Token (connections:write) — required for Socket Mode inbound
# HERALD_SLACK_CHANNEL_ID=C0123456789    # default outbound channel id
# --- Max (Wave 8 — reserved) ---
# HERALD_MAX_BOT_TOKEN=
# --- Email (Wave 8 — reserved) ---
# HERALD_EMAIL_IMAP_HOST=
# HERALD_EMAIL_IMAP_PORT=993
# HERALD_EMAIL_SMTP_HOST=
# HERALD_EMAIL_FROM=
# --- Multi-channel selector ---
# HERALD_CHANNELS=tgram,slack            # comma-separated; default tgram
```

- [ ] **Step 4: Migrate HRD-110..HRD-121 atomically to Fixed.md**

For each HRD-110..HRD-121, move its row from `docs/Issues.md` (`## Open` / `In progress`) to `docs/Fixed.md` with status `fixed`, a closing date `2026-05-27`, and a one-line evidence pointer (the test name / qa dir / e2e invariant that proves it). Update the `docs/Issues.md` metadata `Issues`/`Fixed` summary rows + Revision bump. Update `docs/Fixed.md` metadata. This is the §11.4.19 atomic migration — Issues and Fixed change in the same commit.

- [ ] **Step 5: Refresh CLAUDE.md + Status.md + CONTINUATION.md**

- `CLAUDE.md`: bump Revision r11→r12; update Status summary to record Wave 7 (generic messenger framework + Slack 2nd impl + multi-channel listen + per-channel inbox + 17th workspace module note if slack adds one — it does NOT, slack lives in commons_messaging/channels/slack/, same module). Update the `Continuation` row to point at Wave 8 (Max/Email adapters). Add the Wave 7 entry to the workspace prose (still 16 modules; channels is a sub-package of commons_messaging).
- `docs/Status.md`: add the Wave 7 status line.
- `docs/CONTINUATION.md`: replace the §3 "Active work" Wave 7 entry with the v0.6.0 tag handoff (operator runs live Slack + Telegram round-trips, commits `docs/qa/HRD-115-<run-id>/` + `docs/qa/HRD-120/`, then tags v0.6.0).

- [ ] **Step 6: Run the full gate suite before tagging**

Run:
```bash
go build ./commons/... ./commons_messaging/... ./pherald/... ./qaherald/...
go test -race -count=1 ./commons_messaging/... ./pherald/... ./qaherald/...
bash tests/test_constitution_inheritance.sh
bash tests/test_wave7_mutation_meta.sh
bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -20
```
Expected: all build + test PASS; inheritance gate PASS (confirm the slack-go `.gitmodules` entry did NOT trip I6); mutation gate 4/4 PASS; e2e new invariants PASS or honest SKIP. ANY FAIL blocks the tag.

- [ ] **Step 7: Commit the docs/release-prep**

Run: `grep -rEn 'MUTATED W7-|// always pass' . --include=*.go || echo clean`
```bash
git add docs/guides/MESSENGER_CHANNELS.md quickstart/.env.example \
        docs/Issues.md docs/Fixed.md docs/Status.md docs/CONTINUATION.md CLAUDE.md
git commit -m "$(cat <<'EOF'
Wave 7 T12 (HRD-121): MESSENGER_CHANNELS guide + Issues→Fixed + release prep

docs/guides/MESSENGER_CHANNELS.md (channels.Channel contract, registry,
per-channel inbox, Telegram + Slack setup, add-a-channel checklist).
HRD-110..HRD-121 migrated atomically to Fixed.md. .env.example reserves
Slack/Max/Email + HERALD_CHANNELS. CLAUDE.md r11→r12. v0.6.0 tag deferred to
operator live-evidence run (§107.x) per CONTINUATION §3.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 8: Tag v0.6.0 (operator-gated) + detached 4-mirror push**

The tag is operator-gated on live evidence per §107.x — do NOT tag from the agent unless the operator has committed `docs/qa/HRD-115-<run-id>/` (live Slack) + the live Telegram transcript. When evidence is present:
```bash
git tag -a v0.6.0 -m "Wave 7: generic messenger-channel framework (Telegram→Channel interface + Slack 2nd impl)"
```
Push runs detached per §11.4.88 (release the commit-lock immediately; push in background). Push the slack-go submodule's origin FIRST (so the submodule SHA the superproject references is reachable on every mirror), THEN push the superproject + tag to all four mirrors:
```bash
# submodule origin first
git -C submodules/slack-go push origin HEAD || true   # only if Herald owns a fork; else skip (upstream tag already public)
# superproject + tag to 4 mirrors (detached)
nohup bash -c 'for m in GitHub GitLab GitFlic GitVerse; do . upstreams/$m.sh; git push "$UPSTREAMABLE_REPOSITORY" HEAD:main --tags; done' >/tmp/wave7_push.log 2>&1 &
disown
```
NOTE: slack-go is a public upstream submodule (not a Herald fork), so there is no submodule-origin push to make — the public `v0.16.0` tag is already reachable. Only push if Herald maintains its own fork. Confirm with the operator.

- [ ] **Step 9: Final self-review**

Confirm: all 12 HRDs in Fixed.md; e2e suite has E81-E88; mutation gate is wired into the release checklist; MESSENGER_CHANNELS.md has the add-a-channel checklist; no `MUTATED W7-` residue anywhere; `git status` clean.

---

## Self-Review (run after writing — done at authoring time)

**1. Spec coverage.** Every locked design decision maps to a task: (1) interface→T1, (2) method set→T1, (3) per-channel inbox→T3, (4) HERALD_CHANNELS→T5, (5) registry→T2, (6) generalized self-filter→T4, (7) Slack 2nd impl→T6 + qaherald T7; Max/Email reserved→T8 spec + T12 env, (8) no MTProto→Socket Mode in T6, (9) §11.4.85 stress+chaos→T11. Spec §11.0/§32.2 update→T8. e2e→T9. mutation gate→T10. docs/release→T12.

**2. Placeholder scan.** No "TBD"/"implement later" in code steps — every Go step shows full code; the only deliberately-deferred items (Max/Email adapters, slack-go field-name confirmation against the pinned tag, per-channel-resilience supervised restart) are explicitly flagged as Wave 8 follow-ups / operator-confirmation points, not silent gaps.

**3. Type consistency.** `channels.Channel` (T1) is satisfied by tgram (T1) + slack (T6) + the test `fakeChannel` (T2). `channels.SelfIdentity{Kind,Value}` + `IdentityKind` constants are consistent T1↔T4↔T6. `channels.Config` (T2) fields (Token/AppToken/Target/BaseURL/Extra) are consumed by tgram init (T2) + slack init (T6) + listen.go (T5). `inbound.Replier` generic signature `(ctx, commons.Recipient, string, string, []commons.Attachment) (string, error)` is consistent T5↔tgram.SendReplyGeneric(T1)↔slack.SendReply(T6)↔channelRouter(T5). `channels.WriteContentAddressed`/`InboxDir`/`MimeToExt` (T3) are used by tgram(T3) + slack(T6). `recordingReplier.onReply` hook is added in T5 and reused in T11.

**Known operator-confirmation points (surfaced, not resolved):**
- T6: slack-go Socket Mode field names (`inner.Files`, `socketmode.EventTypeEventsAPI`) must be verified against the pinned `v0.16.0` tag before implementation; the Subscribe file-download path may need adjustment.
- T6/T12: the `.gitmodules` slack-go entry must not trip inheritance gate I6 — verify `tests/test_constitution_inheritance.sh` passes; if I6 forbids ANY `.gitmodules`, vendor slack-go via a sibling-clone `replace` instead and surface to operator.
- T11: the chaos test pins fail-loud-on-channel-death (errgroup propagation). Per-channel supervised restart is a deliberate Wave 8 design decision, not implemented here.

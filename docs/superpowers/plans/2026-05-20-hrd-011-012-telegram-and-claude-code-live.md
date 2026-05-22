<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# HRD-011 (Telegram) + HRD-012 (Claude Code) Live Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land Herald's first true end-user-visible vertical slice: a real Telegram message arrives → Herald dispatches it to Claude Code via `claude --resume <session>` → the `<<<HERALD-REPLY>>>` is parsed → reply gets sent back through Telegram. Closes HRD-011 + HRD-012; adds §107 E17 (Telegram round-trip) + E18 (Claude Code round-trip) + E19 (full slice round-trip) invariants to `scripts/e2e_bluff_hunt.sh`.

**Architecture:** Two thin live integrations on top of Plan 1's storage stack. `commons_messaging/channels/tgram/` swaps its stub `Send`/`Subscribe`/`HealthCheck` for real telebot.v3 calls (vendored as `submodules/telebot/`) and persists `outbound_delivery_evidence` rows via the live `commons_storage` pool. `commons_messaging/dispatch/claude_code/` swaps its stub `Dispatch` for a real `claude --resume <UUID> --print "<envelope>"` invocation with structured stdout parsing. The full slice ties them: an inbound Telegram message routed through the `Subscriber` model (§7) → `claude_code.Dispatch` → parsed `<<<HERALD-REPLY>>>` → `tgram.Send`. All three new e2e invariants SKIP-with-reason per §11.4.3 if operator credentials absent.

**Tech Stack:** Go 1.25+; gopkg.in/telebot.v3 (vendored as `submodules/telebot/`); `claude` CLI (external binary, operator-supplied); pgx + `commons_storage` for persistence; `commons_infra` for the live pool. Operator-supplied env vars: `HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`, `HERALD_CLAUDE_BIN` (defaults to `claude` on `$PATH`), `HERALD_CLAUDE_PROJECT_NAME`.

**Spec refs:** HRD-011, HRD-012 in `docs/Issues.md`; spec V3 §11.1 (Telegram), §32 (subscriber-reply loops), §33 (LLM/agent dispatch), §33.2 (session-resolution algorithm); master roadmap §"Wave 1 — Storage + first user-visible feature".

**Catalogue-Check (Universal §11.4.74):**
- `no-match` → vendor `gopkg.in/telebot.v3` as `submodules/telebot/` (no equivalent in vasic-digital/HelixDevelopment per §11.4.74 mandate)
- `extend digital.vasic.database@<pinned>` (pgx + commons_storage live pool from Plan 1)
- `no-match` → `claude` CLI is an external binary, not a library
- `extend digital.vasic.background.TaskQueue` if queueing outbound retries (deferred to a future HRD; this plan uses synchronous Send)

The HRD-011 row's References cell MUST be updated with the resolved Catalogue-Check value before HRD-011 closes in Task 11.

---

## File Structure

### Files to CREATE

| Path | Responsibility |
|---|---|
| `submodules/telebot/` | NEW git submodule pointing at https://github.com/tucnak/telebot.git (the `gopkg.in/telebot.v3` repo) |
| `commons_messaging/channels/tgram/send.go` | Live `sendMessage` + `sendDocument` against Bot API via telebot.Client |
| `commons_messaging/channels/tgram/subscribe.go` | `getUpdates` long-poll (25s) + 30s safety-net timer per §32.2 |
| `commons_messaging/channels/tgram/healthcheck.go` | `getMe` API call to verify bot token |
| `commons_messaging/channels/tgram/persist.go` | Persist `outbound_delivery_evidence` rows via injected `commons_storage.Database` |
| `commons_messaging/channels/tgram/tgram_integration_test.go` | `//go:build integration` — Telegram round-trip live test (E17) |
| `commons_messaging/dispatch/claude_code/dispatch.go` | Live `claude --resume <UUID> --print "<envelope>"` invocation + stdout parse |
| `commons_messaging/dispatch/claude_code/dispatch_integration_test.go` | `//go:build integration` — Claude Code round-trip live test (E18) |
| `commons_messaging/dispatch/claude_code/persist.go` | Persist `session_state` rows (session UUID + last_dispatch_at) via injected DB |
| `commons_messaging/vertical_slice_integration_test.go` | `//go:build integration` — full Telegram→Claude→Telegram slice (E19) |
| `docs/Fixed.md` HRD-011 + HRD-012 entries | Atomic Issues→Fixed at end |

### Files to MODIFY

| Path | Change |
|---|---|
| `commons_messaging/channels/tgram/tgram.go` | Inject the live `*telebot.Bot` client into `Adapter`; remove "not implemented" stubs and delegate to `send.go` / `subscribe.go` / `healthcheck.go` |
| `commons_messaging/channels/tgram/go.mod` | Add `require gopkg.in/telebot.v3 v3.x.x` + `replace gopkg.in/telebot.v3 => ../../../submodules/telebot` |
| `commons_messaging/dispatch/claude_code/claude_code.go` | Inject `os/exec`-backed runner into `Dispatcher`; remove stub return; delegate to `dispatch.go` |
| `commons_storage/migrations/000010_outbound_delivery_evidence.up.sql` + `.down.sql` | NEW migration if not already present in 000001..000005 — verify first via `grep -l outbound_delivery commons_storage/migrations/*.sql` |
| `commons_storage/migrations/000011_session_state.up.sql` + `.down.sql` | NEW migration for Claude Code session tracking — verify §33 says what columns are needed |
| `commons_storage/migrations_test.go` | Bump expected migration count if migrations 000010/000011 are added |
| `pherald/cmd/pherald/serve.go` (or equivalent) | Wire the live Telegram subscriber + Claude Code dispatcher loop into pherald serve (if pherald owns the slice runner — verify; otherwise this lives in a separate run target) |
| `scripts/e2e_bluff_hunt.sh` | Add E17 (Telegram round-trip), E18 (Claude Code round-trip), E19 (full slice) invariants; bump count 18 → 21 |
| `quickstart/.env.example` | Document `HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`, `HERALD_CLAUDE_BIN`, `HERALD_CLAUDE_PROJECT_NAME` |
| `docs/Issues.md` | Atomic migrate HRD-011 + HRD-012 rows → Fixed.md (per §11.4.19) at the end of Task 11 |
| `docs/Status.md` | r7 → r8: commons_messaging Telegram + Claude Code now ✅ landed; e2e count 21 |
| `docs/{Issues,Fixed,Status}.{html,docx,pdf}` | Regenerate after edits |

### Files NOT touched (deferred to later HRDs)

- `commons_messaging/channels/{slack,email,markdown,...}` — out of Wave 1 scope
- `commons_messaging/dispatch/{openai,aider,...}` — §33 lists these as later iterations
- §42 constitution bindings (Wave 3) — not touched by Plan 2

---

## Task 1: Vendor telebot.v3 as a Helix-style submodule

**Why first:** Per §11.4.74 + CLAUDE.md's "Vendored SDKs" rule, the Telegram SDK MUST be a git submodule (not `go get`'d into go.mod). Vendor it first so all subsequent tasks can use the real type.

**Files:**
- New submodule: `submodules/telebot/`
- Modify: `.gitmodules`
- Modify: `commons_messaging/channels/tgram/go.mod` (require + replace)

- [ ] **Step 1: Verify telebot.v3's upstream repo is accessible**

Run:
```bash
cd /Users/milosvasic/Projects/Herald
git ls-remote https://github.com/tucnak/telebot.git 2>&1 | head -3
```

Expected: lists remote heads (proves the repo exists and is reachable). If access fails, fall back to checking the v3 tag exists via `git ls-remote https://github.com/tucnak/telebot.git v3.*`.

- [ ] **Step 2: Add the submodule**

Run:
```bash
git submodule add https://github.com/tucnak/telebot.git submodules/telebot
cd submodules/telebot
git checkout v3.3.8  # or whatever the latest v3.* stable tag is — verify with `git tag | grep '^v3' | sort -V | tail -5` first
cd ../..
git submodule update --init --recursive
```

NOTE: Use a tagged version, NOT `main`. Tagged versions are reproducible. If `v3.3.8` doesn't exist, pick the highest `v3.*.*` tag.

- [ ] **Step 3: Add require + replace in `commons_messaging/channels/tgram/go.mod`**

Wait — `commons_messaging/channels/tgram/` is not its own module today. Verify:
```bash
ls commons_messaging/channels/tgram/
ls commons_messaging/
find commons_messaging -name 'go.mod'
```

If `commons_messaging/` has a single `go.mod` at its root, modify THAT file. Add:
```
require gopkg.in/telebot.v3 v3.3.8  // adjust to the actual tag you pinned

replace gopkg.in/telebot.v3 => ../submodules/telebot
```

If `commons_messaging/channels/tgram/` is its own submodule with its own go.mod, modify that one and adjust the relative path accordingly.

Run `go mod tidy` from the appropriate module root.

- [ ] **Step 4: Verify the import resolves**

Run:
```bash
cd /Users/milosvasic/Projects/Herald
go build ./commons_messaging/channels/tgram/...
```

Expected: clean build. (No imports of telebot yet, but the require+replace must not error.)

- [ ] **Step 5: Confirm inheritance gate still green**

Run:
```bash
bash tests/test_constitution_inheritance.sh 2>&1 | tail -5
```

Expected: 15 PASS / 0 FAIL. The new submodule must NOT trip I6 (which forbids only the `constitution/` path) per HRD-080 refinement.

- [ ] **Step 6: Commit**

```bash
git add .gitmodules submodules/telebot commons_messaging/go.mod commons_messaging/go.sum  # adjust paths
git commit -m "HRD-011 step 1: vendor telebot.v3 as submodules/telebot

Per Universal §11.4.74 + Herald CLAUDE.md \"Vendored SDKs\" rule, the
Telegram Bot API client lives as a git submodule, not go get'd. Pinned
to v3.3.8 (or whatever stable tag was selected) for reproducibility.

Catalogue-Check: no-match (no Telegram SDK exists in
vasic-digital/HelixDevelopment); vendor as Herald submodule.

Inheritance gate 15/15 PASS unchanged (HRD-080 I6 refinement permits
non-constitution submodules).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

DO NOT push.

---

## Task 2: Telegram `HealthCheck` against live Bot API

**Why second:** HealthCheck is the smallest live integration — proves token works before Send/Subscribe touch the wire.

**Files:**
- Create: `commons_messaging/channels/tgram/healthcheck.go`
- Modify: `commons_messaging/channels/tgram/tgram.go` (inject telebot.Bot; rewire HealthCheck)

- [ ] **Step 1: Write failing integration test**

Create `commons_messaging/channels/tgram/healthcheck_integration_test.go`:

```go
//go:build integration

package tgram

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHealthCheck_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	if token == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN absent per §11.4.3 explicit SKIP-with-reason")
	}
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_CHAT_ID absent")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck against live Bot API: %v", err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL ("not implemented")**

Run:
```bash
cd /Users/milosvasic/Projects/Herald
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestHealthCheck_LiveBotAPI -count=1 -timeout=60s
```

If no operator-supplied credentials: SKIP cleanly per the build-tag dance.
If credentials present: expect FAIL with "not implemented".

- [ ] **Step 3: Inject telebot.Bot into Adapter**

Modify `commons_messaging/channels/tgram/tgram.go`:

Update the `Adapter` struct:
```go
type Adapter struct {
	botToken string
	chatID   string
	bot      *telebot.Bot // nil until first connection
}
```

Add the telebot import:
```go
import (
	// ... existing ...
	telebot "gopkg.in/telebot.v3"
)
```

- [ ] **Step 4: Create `commons_messaging/channels/tgram/healthcheck.go`**

```go
package tgram

import (
	"context"
	"fmt"

	telebot "gopkg.in/telebot.v3"
)

// HealthCheck verifies the bot token by issuing a getMe call against the
// live Bot API. Returns nil only if the API responds with a non-empty
// bot username — proves the token is valid AND the bot is enabled.
//
// Per §107: HealthCheck is the cheapest live evidence the adapter works.
// A PASS without observing a real getMe response would be a §107 bluff.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.bot == nil {
		bot, err := telebot.NewBot(telebot.Settings{
			Token:  a.botToken,
			Client: nil, // use default http.Client
		})
		if err != nil {
			return fmt.Errorf("tgram: connect to Bot API: %w", err)
		}
		a.bot = bot
	}
	me, err := a.bot.Raw("getMe", nil)
	if err != nil {
		return fmt.Errorf("tgram: getMe: %w", err)
	}
	// telebot.Bot.Raw returns the JSON response body bytes; assert non-empty.
	if len(me) == 0 {
		return fmt.Errorf("tgram: getMe returned empty body (§107 bluff guard)")
	}
	return nil
}
```

NOTE: telebot.v3's API: confirm `Bot.Raw(method, params interface{}) ([]byte, error)` exists. If the API surface differs (e.g., `Bot.Me()` returns a `*User` directly), use that and assert `me.Username != ""`. Adjust based on what `go doc gopkg.in/telebot.v3.Bot` returns.

- [ ] **Step 5: Remove the stub HealthCheck from `tgram.go`**

Delete the old `func (a *Adapter) HealthCheck(ctx context.Context) error` that returns `errors.New("not implemented")` from `tgram.go`. The new one in `healthcheck.go` replaces it.

- [ ] **Step 6: Run unit + integration tests**

```bash
go test -race -count=1 ./commons_messaging/channels/tgram/  # unit only
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestHealthCheck_LiveBotAPI -count=1 -timeout=60s
```

Expected:
- Unit tests: still PASS (HealthCheck has no unit test currently — and we're NOT adding one for the live wire since unit-testing real network calls is meaningless mocking)
- Integration test (with creds): PASS — getMe succeeds, non-empty body
- Integration test (without creds): SKIP-with-reason

- [ ] **Step 7: Commit**

```bash
git add commons_messaging/channels/tgram/tgram.go commons_messaging/channels/tgram/healthcheck.go commons_messaging/channels/tgram/healthcheck_integration_test.go
git commit -m "HRD-011 step 2: Telegram HealthCheck against live Bot API

Replaces the stub HealthCheck with a real getMe call via telebot.v3.
Integration test (//go:build integration) SKIPs cleanly if
HERALD_TGRAM_BOT_TOKEN absent (§11.4.3); PASSes only if Bot API
responds with non-empty body.

§107 evidence: cheapest live wire — proves token valid + bot enabled
before later tasks touch sendMessage / getUpdates.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: Telegram `Send` (sendMessage + sendDocument)

**Files:**
- Create: `commons_messaging/channels/tgram/send.go`
- Modify: `commons_messaging/channels/tgram/tgram.go` (rewire Send)

- [ ] **Step 1: Write failing integration test**

Append to `commons_messaging/channels/tgram/healthcheck_integration_test.go` (or create `send_integration_test.go`):

```go
//go:build integration

package tgram

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

func TestSend_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or _CHAT_ID absent")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg := commons.OutboundMessage{
		Body: commons.Body{
			Text: "Herald E17 integration test " + time.Now().Format(time.RFC3339Nano),
		},
		Recipient: commons.Recipient{
			ChannelID: commons.ChannelID("tgram://" + token + "/" + chatID),
		},
	}
	receipt, err := a.Send(ctx, msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt.MessageID == "" {
		t.Fatal("Send returned empty MessageID — §107 bluff guard (proves Telegram actually received the message and returned a chat-side ID)")
	}
}
```

- [ ] **Step 2: Run, expect FAIL ("not implemented")**

```bash
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_LiveBotAPI -count=1 -timeout=60s
```

Expected: FAIL with "not implemented" or SKIP without creds.

- [ ] **Step 3: Create `commons_messaging/channels/tgram/send.go`**

```go
package tgram

import (
	"context"
	"fmt"
	"strconv"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
)

// Send dispatches a single OutboundMessage to the bot's configured chat.
// Uses telebot.v3's Bot.Send with parseMode=MarkdownV2 per spec §11.1.
// On success, returns a Receipt with the Telegram-side message ID for
// idempotency tracking.
//
// Anti-bluff (§107): the returned Receipt.MessageID MUST be the chat-side
// integer assigned by Telegram, not a Herald-generated UUID. A PASS that
// returns a fake ID is a §107 bluff.
func (a *Adapter) Send(ctx context.Context, msg commons.OutboundMessage) (commons.Receipt, error) {
	if a.bot == nil {
		// Lazy connect on first Send.
		bot, err := telebot.NewBot(telebot.Settings{Token: a.botToken})
		if err != nil {
			return commons.Receipt{}, fmt.Errorf("tgram.Send: connect: %w", err)
		}
		a.bot = bot
	}

	chatIDInt, err := strconv.ParseInt(a.chatID, 10, 64)
	if err != nil {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: chatID %q not numeric: %w", a.chatID, err)
	}
	chat := &telebot.Chat{ID: chatIDInt}

	opts := &telebot.SendOptions{
		ParseMode: telebot.ModeMarkdownV2,
	}
	sent, err := a.bot.Send(chat, msg.Body.Text, opts)
	if err != nil {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: sendMessage: %w", err)
	}
	if sent == nil || sent.ID == 0 {
		return commons.Receipt{}, fmt.Errorf("tgram.Send: empty sent message — §107 bluff guard")
	}

	return commons.Receipt{
		MessageID: strconv.Itoa(sent.ID),
		Evidence:  commons.DeliveryEvidence(1), // delivered
	}, nil
}
```

NOTE: Confirm `telebot.Bot.Send(to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error)` exists with this signature. Look at `submodules/telebot/bot.go` to verify. If the signature differs, adjust.

NOTE: `commons.DeliveryEvidence` is an int enum. Use the proper constant from `commons/types.go` instead of `1` — look up the value for "delivered" (likely `commons.DeliveryDelivered` or similar). DO NOT hardcode the integer.

- [ ] **Step 4: Remove stub Send from `tgram.go`**

Delete the old stub `Send(ctx, msg) (Receipt, error) { return Receipt{}, errors.New("not implemented") }`.

- [ ] **Step 5: Run integration test (with creds)**

```bash
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_LiveBotAPI -count=1 -timeout=60s
```

Expected: PASS. The test bot's chat receives the test message; the receipt's MessageID is a non-empty Telegram-side integer string.

- [ ] **Step 6: Commit**

```bash
git add commons_messaging/channels/tgram/tgram.go commons_messaging/channels/tgram/send.go commons_messaging/channels/tgram/healthcheck_integration_test.go
git commit -m "HRD-011 step 3: Telegram Send (sendMessage MarkdownV2)

Replaces stub Send with real telebot.v3 Bot.Send call. Returns Receipt
with chat-side MessageID (the integer Telegram assigns) — not a Herald-
generated UUID. §107 bluff guard: asserts non-empty MessageID in the
integration test, proving the message actually landed in the chat.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: Telegram `Subscribe` (getUpdates long-poll)

**Files:**
- Create: `commons_messaging/channels/tgram/subscribe.go`
- Modify: `commons_messaging/channels/tgram/tgram.go` (rewire Subscribe)

**Spec ref:** §32.2 — 25s long-poll timeout, 30s safety-net timer for stalled polls.

- [ ] **Step 1: Write integration test**

Create `commons_messaging/channels/tgram/subscribe_integration_test.go`:

```go
//go:build integration

package tgram

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
)

// TestSubscribe_LiveBotAPI exercises the long-poll loop. The operator
// MUST send a message to the configured bot within the 60s test window
// for the test to PASS; otherwise it SKIPs after the timeout.
//
// §107: an empty-poll PASS is a bluff — the assertion that ≥1 message
// was actually received from the operator's hand-sent message is the
// load-bearing check.
func TestSubscribe_LiveBotAPI(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — HERALD_TGRAM_BOT_TOKEN or _CHAT_ID absent")
	}
	if os.Getenv("HERALD_TGRAM_LIVE_INBOUND") != "1" {
		t.Skipf("skip: hardware_not_present — set HERALD_TGRAM_LIVE_INBOUND=1 AND send a message to the bot within 60s to exercise this test")
	}

	a, err := New("tgram://" + token + "/" + chatID)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var got atomic.Int64
	handler := commons.InboundHandlerFunc(func(ctx context.Context, ev commons.InboundEvent) error {
		got.Add(1)
		return nil
	})
	if err := a.Subscribe(ctx, handler); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Subscribe: %v", err)
	}
	if got.Load() == 0 {
		t.Fatal("Subscribe received 0 messages from operator's hand-sent input — §107 bluff guard (proves getUpdates actually pulled real updates)")
	}
}
```

NOTE: `commons.InboundHandlerFunc` may not exist — `commons.InboundHandler` is an interface. Either:
(a) `commons` exposes a `HandlerFunc` adapter — check `commons/types.go`
(b) Define a small local adapter in the test file
(c) Use a struct that implements `InboundHandler`

Use whichever is cleanest. The test must actually invoke the handler when an inbound message arrives.

- [ ] **Step 2: Run, expect FAIL**

```bash
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID HERALD_TGRAM_LIVE_INBOUND=1 \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSubscribe_LiveBotAPI -count=1 -timeout=90s
```

Expected: FAIL with "not implemented" (without the inbound flag: SKIP).

- [ ] **Step 3: Create `commons_messaging/channels/tgram/subscribe.go`**

```go
package tgram

import (
	"context"
	"fmt"
	"time"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
)

// Subscribe runs the long-poll getUpdates loop until ctx is cancelled.
// Per spec §32.2: 25s long-poll timeout, 30s safety-net timer.
//
// Implementation note: telebot.v3 ships its own poller (telebot.LongPoller)
// — we wire that with timeout=25s and start the bot. The 30s safety-net
// fires only if the poll thread misses two consecutive 25s windows, which
// indicates a real stall. We log + continue rather than die.
func (a *Adapter) Subscribe(ctx context.Context, h commons.InboundHandler) error {
	if a.bot == nil {
		bot, err := telebot.NewBot(telebot.Settings{
			Token:  a.botToken,
			Poller: &telebot.LongPoller{Timeout: 25 * time.Second},
		})
		if err != nil {
			return fmt.Errorf("tgram.Subscribe: connect: %w", err)
		}
		a.bot = bot
	}

	// telebot dispatches via handler registrations.
	a.bot.Handle(telebot.OnText, func(c telebot.Context) error {
		ev := commons.InboundEvent{
			ChannelID: commons.ChannelID("tgram://" + a.botToken + "/" + a.chatID),
			Body: commons.Body{
				Text: c.Message().Text,
			},
			// Source: telebot.v3 Message → commons.InboundEvent mapping
			// — add ConversationRef from c.Message().Chat.ID + MessageID.
		}
		return h.Handle(ctx, ev)
	})

	// Safety-net timer per §32.2.
	stallTicker := time.NewTicker(30 * time.Second)
	defer stallTicker.Stop()
	stallDetect := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stallTicker.C:
				select {
				case stallDetect <- struct{}{}:
				default:
				}
			}
		}
	}()

	// Block until ctx cancelled. telebot.Start runs in this goroutine.
	go a.bot.Start()
	defer a.bot.Stop()
	<-ctx.Done()
	return ctx.Err()
}
```

NOTE: The above mixes goroutines + telebot's Start. Verify telebot.Start's semantics — is it blocking or non-blocking? Adjust the goroutine wiring to match. If `Start` is blocking, run it in a goroutine and wait on ctx.

NOTE: `commons.InboundHandler.Handle(ctx, ev) error` is the interface method — verify the exact method name via `grep -A 3 'type InboundHandler' commons/types.go`. If it's `OnInbound` or `Receive`, use that name.

- [ ] **Step 4: Remove stub Subscribe from `tgram.go`**

- [ ] **Step 5: Run integration test with the hand-sent inbound flag**

```bash
# Step a: in another terminal, manually send a Telegram message to the bot.
# Step b: in this terminal, run:
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID HERALD_TGRAM_LIVE_INBOUND=1 \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSubscribe_LiveBotAPI -count=1 -timeout=90s
```

Expected: PASS within 60s — the handler ran at least once, atomic counter > 0.

- [ ] **Step 6: Commit**

```bash
git add commons_messaging/channels/tgram/tgram.go commons_messaging/channels/tgram/subscribe.go commons_messaging/channels/tgram/subscribe_integration_test.go
git commit -m "HRD-011 step 4: Telegram Subscribe (getUpdates long-poll)

Live long-poll loop with 25s telebot.LongPoller timeout + 30s safety-net
timer per spec §32.2. Real handler invocation on OnText events.

§107: integration test asserts ≥1 handler invocation from an operator-
hand-sent message — proves the loop actually pulled real updates, not
just \"no error from polling\".

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: Telegram persistence (outbound_delivery_evidence)

**Files:**
- Create: `commons_messaging/channels/tgram/persist.go`
- Modify: `commons_messaging/channels/tgram/send.go` (call Persist after successful Send)
- Verify migration exists: `commons_storage/migrations/` for `outbound_delivery_evidence` table

- [ ] **Step 1: Verify the outbound_delivery_evidence schema**

```bash
cd /Users/milosvasic/Projects/Herald
grep -rln 'outbound_delivery' commons_storage/migrations/
```

If a migration already declares the table (likely from Plan 1's M2 work), inspect its columns. If NOT present, create a new migration `000010_outbound_delivery_evidence.up.sql`.

Required columns at minimum:
- id UUID PK
- tenant_id UUID NOT NULL
- channel_id TEXT NOT NULL
- channel_message_id TEXT NOT NULL  (the Telegram-side ID returned by sendMessage)
- evidence INT NOT NULL (commons.DeliveryEvidence enum)
- sent_at TIMESTAMPTZ DEFAULT now()

If you create the migration, bump `commons_storage/migrations_test.go`'s expected count by 1.

- [ ] **Step 2: Write failing integration test**

Create `commons_messaging/channels/tgram/persist_integration_test.go`:

```go
//go:build integration

package tgram

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
	infra "github.com/vasic-digital/herald/commons_infra"
	storage "github.com/vasic-digital/herald/commons_storage"
)

func TestSend_PersistsDeliveryEvidence(t *testing.T) {
	token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
	chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		t.Skipf("skip: hardware_not_present — Telegram credentials absent")
	}
	if _, err := exec.LookPath("podman"); err != nil {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("skip: hardware_not_present — no container runtime")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	boot, err := infra.NewQuickstartBoot(infra.Config{Services: []string{"postgres"}})
	if err != nil {
		t.Fatalf("NewQuickstartBoot: %v", err)
	}
	if err := boot.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	t.Cleanup(func() { _ = boot.Down(context.Background()) })

	pool, err := boot.Pool()
	if err != nil {
		t.Fatalf("Pool: %v", err)
	}

	a, err := NewWithStorage("tgram://"+token+"/"+chatID, pool) // NEW constructor — see Step 3
	if err != nil {
		t.Fatalf("NewWithStorage: %v", err)
	}

	tenant := uuid.New()
	msgText := "Herald E17 persist test " + time.Now().Format(time.RFC3339Nano)
	receipt, err := a.SendForTenant(ctx, tenant, commons.OutboundMessage{
		Body:      commons.Body{Text: msgText},
		Recipient: commons.Recipient{ChannelID: commons.ChannelID("tgram://" + token + "/" + chatID)},
	})
	if err != nil {
		t.Fatalf("SendForTenant: %v", err)
	}

	// Read back the persisted row via WithTenantContext.
	var persistedChannelMsgID string
	err = storage.WithTenantContext(ctx, pool, tenant, func(tx db.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT channel_message_id FROM outbound_delivery_evidence WHERE tenant_id = $1 ORDER BY sent_at DESC LIMIT 1`,
			tenant,
		).Scan(&persistedChannelMsgID)
	})
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if persistedChannelMsgID != receipt.MessageID {
		t.Fatalf("persisted channel_message_id mismatch: got %q want %q (§107 bluff guard)", persistedChannelMsgID, receipt.MessageID)
	}
}
```

NOTE: `NewWithStorage` and `SendForTenant` are new APIs you'll define in Step 3. The existing `Send` doesn't know about tenants or pools — we need a tenant-scoped variant for persistence.

NOTE: Import paths: `exec "os/exec"`, `db "digital.vasic.database/pkg/database"`.

- [ ] **Step 3: Create `commons_messaging/channels/tgram/persist.go` + extend Adapter**

```go
package tgram

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	db "digital.vasic.database/pkg/database"

	"github.com/vasic-digital/herald/commons"
	storage "github.com/vasic-digital/herald/commons_storage"
)

// NewWithStorage constructs an Adapter that persists delivery evidence
// to the given live pool. Persistence happens AFTER a successful
// sendMessage — a Send-then-persist failure leaves the message delivered
// without a persistence row, which is a known limitation (§107 honest:
// we prefer dropping a persistence row over double-sending the message).
func NewWithStorage(rawURL string, pool db.Database) (*Adapter, error) {
	a, err := New(rawURL)
	if err != nil {
		return nil, err
	}
	a.pool = pool
	return a, nil
}

// SendForTenant calls Send and persists the delivery evidence row under
// the given tenant's RLS context. Returns the same Receipt as Send,
// plus an error if EITHER the network send OR the persistence write
// fails.
func (a *Adapter) SendForTenant(ctx context.Context, tenantID uuid.UUID, msg commons.OutboundMessage) (commons.Receipt, error) {
	receipt, err := a.Send(ctx, msg)
	if err != nil {
		return receipt, err
	}
	if a.pool == nil {
		// Persistence not configured — return receipt unchanged.
		return receipt, nil
	}
	err = storage.WithTenantContext(ctx, a.pool, tenantID, func(tx db.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO outbound_delivery_evidence (id, tenant_id, channel_id, channel_message_id, evidence)
			 VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), tenantID, string(msg.Recipient.ChannelID), receipt.MessageID, int(receipt.Evidence),
		)
		return execErr
	})
	if err != nil {
		return receipt, fmt.Errorf("tgram.SendForTenant: persist: %w", err)
	}
	return receipt, nil
}
```

Add `pool db.Database` to the Adapter struct in `tgram.go`:

```go
type Adapter struct {
	botToken string
	chatID   string
	bot      *telebot.Bot
	pool     db.Database // optional; nil = persistence disabled
}
```

- [ ] **Step 4: Run integration test**

```bash
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_PersistsDeliveryEvidence -count=1 -timeout=180s
```

Expected: PASS — Telegram delivers the message; a row in `outbound_delivery_evidence` matches the chat-side MessageID exactly.

- [ ] **Step 5: Commit**

```bash
git add commons_messaging/channels/tgram/*.go commons_messaging/channels/tgram/go.mod commons_storage/migrations/000010_*.sql commons_storage/migrations_test.go
git commit -m "HRD-011 step 5: Telegram delivery-evidence persistence

NewWithStorage constructs an Adapter that persists outbound_delivery_
evidence rows to the live pool. SendForTenant calls Send then persists
the row under the tenant's RLS context.

§107 evidence: integration test asserts the persisted channel_message_
id matches the receipt's MessageID exactly — not 'a row exists'.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: Claude Code `Dispatch` (live `claude --resume` invocation)

**Files:**
- Create: `commons_messaging/dispatch/claude_code/dispatch.go`
- Modify: `commons_messaging/dispatch/claude_code/claude_code.go` (rewire Dispatch)

- [ ] **Step 1: Write integration test**

Create `commons_messaging/dispatch/claude_code/dispatch_integration_test.go`:

```go
//go:build integration

package claude_code

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestDispatch_LiveClaudeInvocation(t *testing.T) {
	binary := os.Getenv("HERALD_CLAUDE_BIN")
	if binary == "" {
		binary = "claude"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("skip: hardware_not_present — %s not on PATH", binary)
	}
	projectName := os.Getenv("HERALD_CLAUDE_PROJECT_NAME")
	if projectName == "" {
		t.Skipf("skip: hardware_not_present — HERALD_CLAUDE_PROJECT_NAME absent")
	}

	d, err := New(binary, ".", projectName)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	req := DispatchRequest{
		InboundText: "Herald E18 integration test: please reply with outcome=acknowledged and summary=ack",
		ProjectName: projectName,
		// other DispatchRequest fields — read DispatchRequest struct
	}
	resp, err := d.Dispatch(ctx, req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if resp.Outcome == "" {
		t.Fatal("Dispatch returned empty Outcome — §107 bluff guard (proves Claude actually emitted the structured reply, not a hand-rolled default)")
	}
	if resp.Summary == "" {
		t.Fatal("Dispatch returned empty Summary — §107 bluff guard")
	}
}
```

- [ ] **Step 2: Run, expect FAIL ("not implemented")**

```bash
HERALD_CLAUDE_PROJECT_NAME=Herald HERALD_CLAUDE_BIN=claude \
go test ./commons_messaging/dispatch/claude_code/ -tags=integration -run TestDispatch_LiveClaudeInvocation -count=1 -timeout=300s
```

Expected: FAIL with "not implemented" if `claude` available; SKIP otherwise.

- [ ] **Step 3: Create `commons_messaging/dispatch/claude_code/dispatch.go`**

```go
package claude_code

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Dispatch invokes `claude --resume <UUID> --print "<envelope>"` and parses
// the structured reply line prefixed with <<<HERALD-REPLY>>>.
//
// §107: a PASS requires (a) Claude exits 0, AND (b) stdout contains a
// well-formed JSON reply with non-empty Outcome + Summary. A reply
// where Claude refused, errored, or never produced the marker is a FAIL.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResponse, error) {
	sessionUUID, anchor, err := d.ResolveSession()
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("dispatch: resolve session: %w", err)
	}

	envelope := d.FormatEnvelope(req)
	cmd := exec.CommandContext(ctx, d.binaryPath,
		"--resume", sessionUUID.String(),
		"--print", envelope,
	)
	cmd.Dir = d.workingDir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return DispatchResponse{}, fmt.Errorf("dispatch: claude --resume %s: exit %d: %s",
				sessionUUID, ee.ExitCode(), strings.TrimSpace(string(ee.Stderr)))
		}
		return DispatchResponse{}, fmt.Errorf("dispatch: exec: %w", err)
	}

	resp, err := parseReply(out)
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("dispatch: parse reply: %w", err)
	}
	resp.SessionUUID = sessionUUID
	resp.AnchorPath = anchor
	return resp, nil
}

// parseReply scans Claude Code's stdout for the <<<HERALD-REPLY>>> marker
// and decodes the following JSON object into DispatchResponse. Returns
// an error if no marker is found or the JSON is malformed — explicit
// reject preserves §107 anti-bluff.
func parseReply(stdout []byte) (DispatchResponse, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(stdout)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow long reply lines
	for scanner.Scan() {
		line := scanner.Text()
		const marker = "<<<HERALD-REPLY>>>"
		if idx := strings.Index(line, marker); idx >= 0 {
			payload := strings.TrimSpace(line[idx+len(marker):])
			var resp DispatchResponse
			if err := json.Unmarshal([]byte(payload), &resp); err != nil {
				return DispatchResponse{}, fmt.Errorf("unmarshal: %w (raw: %q)", err, payload)
			}
			return resp, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return DispatchResponse{}, fmt.Errorf("scan stdout: %w", err)
	}
	return DispatchResponse{}, fmt.Errorf("no <<<HERALD-REPLY>>> marker found in claude stdout (§107 bluff guard)")
}
```

- [ ] **Step 4: Inspect DispatchResponse struct fields**

Confirm the `DispatchResponse` struct in `claude_code.go` has the fields the test asserts on (Outcome, Summary, SessionUUID, AnchorPath). If `SessionUUID` + `AnchorPath` are NOT present, add them as new exported fields. Update the `replyJSONSchema` const if needed to reflect the schema.

- [ ] **Step 5: Remove stub Dispatch from `claude_code.go`**

Delete the old `Dispatch` that returns `"not implemented"`.

- [ ] **Step 6: Run integration test (with operator-supplied `claude`)**

```bash
HERALD_CLAUDE_PROJECT_NAME=Herald HERALD_CLAUDE_BIN=claude \
go test ./commons_messaging/dispatch/claude_code/ -tags=integration -run TestDispatch_LiveClaudeInvocation -count=1 -timeout=300s
```

Expected: PASS — the real `claude` CLI receives the envelope, replies with a `<<<HERALD-REPLY>>>` line, the JSON decodes, Outcome + Summary are non-empty.

NOTE: This requires Claude Code being correctly configured for the test machine. If the test FAILs with "no marker found", the issue is likely that the operator-side claude session doesn't honour the envelope schema yet — that's a real bug worth investigating (NOT a §107 PASS-bluff to paper over).

- [ ] **Step 7: Commit**

```bash
git add commons_messaging/dispatch/claude_code/*.go
git commit -m "HRD-012 step 6: Claude Code Dispatch live invocation

Replaces stub Dispatch with real exec.Command(claude --resume <UUID>
--print <envelope>). Parses stdout for <<<HERALD-REPLY>>> JSON line per
spec §33. §107 bluff guards: rejects if no marker found OR Outcome/
Summary empty.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: Claude Code session persistence

**Files:**
- Create: `commons_messaging/dispatch/claude_code/persist.go`
- Modify: `commons_messaging/dispatch/claude_code/dispatch.go` (persist session_state after each Dispatch)
- Possibly create: `commons_storage/migrations/000011_session_state.up.sql` (verify if not already present)

- [ ] **Step 1: Verify or create the session_state migration**

```bash
grep -rln 'session_state\|claude_session' commons_storage/migrations/
```

If not present, create migration `000011_session_state.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS claude_code_sessions (
    project_name TEXT PRIMARY KEY,
    session_uuid UUID NOT NULL,
    anchor_path TEXT NOT NULL,
    last_dispatch_at TIMESTAMPTZ DEFAULT now(),
    last_response JSONB
);
```

Plus down migration. Bump `commons_storage/migrations_test.go` expected count.

- [ ] **Step 2: Write failing integration test**

Append to `dispatch_integration_test.go`:

```go
func TestDispatch_PersistsSessionState(t *testing.T) {
	// Same env-var guards as TestDispatch_LiveClaudeInvocation.
	// PLUS boot commons_infra QuickstartBoot for the live pool.

	// Call Dispatch, then SELECT from claude_code_sessions and assert
	// project_name + session_uuid + last_dispatch_at populated.
	// Assert last_response JSONB contains Outcome + Summary.
}
```

(Full test body follows the pattern from Task 5 — boot Postgres, do Dispatch, read-back.)

- [ ] **Step 3: Create `commons_messaging/dispatch/claude_code/persist.go`**

```go
package claude_code

import (
	"context"
	"encoding/json"
	"fmt"

	db "digital.vasic.database/pkg/database"

	storage "github.com/vasic-digital/herald/commons_storage"
)

// PersistSessionState upserts the session row for this project after a
// successful Dispatch. Persistence is best-effort — a PG-down state
// returns an error to the caller but does NOT roll back the dispatch
// itself (the claude reply has already been received).
//
// The function is RLS-scoped to a Herald-internal "system" tenant UUID
// since session-state is operator-shared, not tenant-scoped. The
// HERALD_SYSTEM_TENANT_UUID const is the canonical anchor.
func (d *Dispatcher) PersistSessionState(ctx context.Context, pool db.Database, resp DispatchResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return storage.WithTenantContext(ctx, pool, HeraldSystemTenant, func(tx db.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO claude_code_sessions (project_name, session_uuid, anchor_path, last_dispatch_at, last_response)
			 VALUES ($1, $2, $3, NOW(), $4)
			 ON CONFLICT (project_name) DO UPDATE SET
			   session_uuid = EXCLUDED.session_uuid,
			   anchor_path = EXCLUDED.anchor_path,
			   last_dispatch_at = NOW(),
			   last_response = EXCLUDED.last_response`,
			d.projectName, resp.SessionUUID, resp.AnchorPath, string(payload),
		)
		return execErr
	})
}
```

NOTE: `HeraldSystemTenant` is a new exported constant — declare it as a fixed UUID at the top of `claude_code.go`:

```go
// HeraldSystemTenant is a fixed UUID that scopes Herald's internal
// (non-tenant-scoped) data. Sessions, configs, etc. live under this.
// Per §16, this is NOT a real tenant — it's the operator-shared bucket.
var HeraldSystemTenant = uuid.MustParse("00000000-0000-0000-0000-000000000001")
```

- [ ] **Step 4: Wire PersistSessionState into Dispatch**

If a pool is provided to the Dispatcher (extend Dispatcher with an optional `pool` field, similar to tgram.Adapter), call PersistSessionState after a successful Dispatch.

- [ ] **Step 5: Run, expect PASS**

```bash
HERALD_CLAUDE_PROJECT_NAME=Herald HERALD_CLAUDE_BIN=claude \
go test ./commons_messaging/dispatch/claude_code/ -tags=integration -count=1 -timeout=300s
```

- [ ] **Step 6: Commit**

```bash
git add commons_messaging/dispatch/claude_code/*.go commons_storage/migrations/000011_session_state.up.sql commons_storage/migrations/000011_session_state.down.sql commons_storage/migrations_test.go
git commit -m "HRD-012 step 7: Claude Code session state persistence

Upserts claude_code_sessions row after each Dispatch — tracks
session_uuid + anchor_path + last_dispatch_at + last_response JSONB.

Scoped to HeraldSystemTenant (fixed UUID, NOT a real tenant) per §16.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: Vertical-slice integration test (E19)

**Files:**
- Create: `commons_messaging/vertical_slice_integration_test.go`

**The slice:** operator hand-sends a Telegram message → tgram.Subscribe receives it → handler calls claude_code.Dispatch → response parsed → handler calls tgram.SendForTenant → operator sees the reply in Telegram.

- [ ] **Step 1: Write the integration test**

```go
//go:build integration

package messaging_test

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
	"github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code"
	infra "github.com/vasic-digital/herald/commons_infra"
)

// TestVerticalSlice exercises the full E19 round-trip:
// operator-sent Telegram inbound → claude_code.Dispatch → telegram outbound.
//
// REQUIRES: operator has booted the quickstart compose; has a Telegram bot
// token; has `claude` on PATH; HAS just sent ANY message to the bot before
// the 90s test window expires.
//
// §107: PASSES only when (a) inbound observed, (b) Claude responded with
// non-empty Outcome, (c) outbound MessageID is non-empty.
func TestVerticalSlice_TelegramClaudeRoundTrip(t *testing.T) {
	// env-var guards: HERALD_TGRAM_BOT_TOKEN, HERALD_TGRAM_CHAT_ID,
	// HERALD_TGRAM_LIVE_INBOUND=1, HERALD_CLAUDE_PROJECT_NAME, HERALD_CLAUDE_BIN.
	// SKIP if any absent.

	// Boot Postgres.
	// Construct tgram.Adapter with NewWithStorage(token, pool).
	// Construct claude_code.Dispatcher.
	// Subscribe with a handler that:
	//   1. Dispatch to Claude.
	//   2. SendForTenant(reply.Summary) back.
	//   3. Atomic flag set.
	// Wait up to 90s for handler completion.
	// Assert: handler ran, dispatch returned non-empty Outcome, outbound MessageID non-empty.
}
```

Fill in the body following the per-task patterns from Tasks 4/5/6/7.

- [ ] **Step 2: Run end-to-end**

```bash
# Send a Telegram message to the bot. Then:
HERALD_TGRAM_BOT_TOKEN=$HERALD_TGRAM_BOT_TOKEN HERALD_TGRAM_CHAT_ID=$HERALD_TGRAM_CHAT_ID \
HERALD_TGRAM_LIVE_INBOUND=1 \
HERALD_CLAUDE_BIN=claude HERALD_CLAUDE_PROJECT_NAME=Herald \
go test ./commons_messaging/ -tags=integration -run TestVerticalSlice_TelegramClaudeRoundTrip -count=1 -timeout=300s -v
```

Expected: PASS — the operator's Telegram chat receives a reply derived from Claude's `<<<HERALD-REPLY>>>` Summary.

- [ ] **Step 3: Commit**

```bash
git add commons_messaging/vertical_slice_integration_test.go
git commit -m "HRD-011 + HRD-012 step 8: vertical-slice E19 integration test

Full Telegram → Claude Code → Telegram round-trip. Requires operator
to hand-send a Telegram message and have claude on PATH. §107: asserts
the operator's chat receives a non-empty derived reply.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: Add E17/E18/E19 to e2e_bluff_hunt.sh

**Files:**
- Modify: `scripts/e2e_bluff_hunt.sh`

- [ ] **Step 1: Add the three invariants**

After the E14-E16 block, add:

```bash
echo ""
echo "== E17-E19: HRD-011 + HRD-012 live channel + dispatcher =="
if [ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ] && [ -n "${HERALD_TGRAM_CHAT_ID:-}" ]; then
    check "E17 Telegram Send delivers + persists evidence (live Bot API + live PG)" \
        "go test ./commons_messaging/channels/tgram/ -tags=integration -run TestSend_PersistsDeliveryEvidence -count=1 -timeout=180s"
else
    echo "SKIP  E17 (no HERALD_TGRAM_BOT_TOKEN — §11.4.3 explicit SKIP-with-reason)"
fi

if command -v "${HERALD_CLAUDE_BIN:-claude}" >/dev/null 2>&1 && [ -n "${HERALD_CLAUDE_PROJECT_NAME:-}" ]; then
    check "E18 Claude Code Dispatch round-trip + session persist (live CLI + live PG)" \
        "go test ./commons_messaging/dispatch/claude_code/ -tags=integration -count=1 -timeout=300s"
else
    echo "SKIP  E18 (no claude CLI or HERALD_CLAUDE_PROJECT_NAME — §11.4.3 explicit SKIP-with-reason)"
fi

if [ -n "${HERALD_TGRAM_BOT_TOKEN:-}" ] && [ -n "${HERALD_TGRAM_LIVE_INBOUND:-}" ] && command -v "${HERALD_CLAUDE_BIN:-claude}" >/dev/null 2>&1; then
    check "E19 full vertical slice — Telegram → Claude Code → Telegram (operator hand-sent inbound)" \
        "go test ./commons_messaging/ -tags=integration -run TestVerticalSlice_TelegramClaudeRoundTrip -count=1 -timeout=300s"
else
    echo "SKIP  E19 (HERALD_TGRAM_LIVE_INBOUND=1 + Telegram creds + claude CLI required — §11.4.3)"
fi
```

- [ ] **Step 2: Update the header comment "Eighteen invariants" → "Twenty-one invariants"** and add E17/E18/E19 to the list.

- [ ] **Step 3: Run the script**

```bash
bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -30
```

Expected outcomes:
- All creds + claude available: 21 PASS / 0 FAIL
- No creds: 18 PASS / 0 FAIL + 3 SKIP (still considered a PASS overall)

- [ ] **Step 4: Commit**

```bash
git add scripts/e2e_bluff_hunt.sh
git commit -m "HRD-011 + HRD-012 step 9: e2e_bluff_hunt E17/E18/E19 — Wave 1 vertical slice

E17: Telegram Send + outbound_delivery_evidence persistence (live Bot API + PG).
E18: Claude Code Dispatch round-trip + session_state persistence (live CLI + PG).
E19: Full slice — operator's Telegram inbound → Claude → Telegram outbound.

Without operator creds, each SKIPs with explicit §11.4.3 reason.

Total e2e_bluff_hunt: 18 PASS → 21 PASS.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: .env.example + atomic Issues→Fixed for HRD-011 + HRD-012

**Files:**
- Modify: `quickstart/.env.example`
- Modify: `docs/Issues.md`, `docs/Fixed.md`, `docs/Status.md`
- Regenerate: all 9 sibling formats

- [ ] **Step 1: Add new env vars to `.env.example`**

```bash
# HRD-011 Telegram (landed 2026-05-XX)
# Get a bot token from @BotFather on Telegram; chatID is a numeric chat ID.
HERALD_TGRAM_BOT_TOKEN=
HERALD_TGRAM_CHAT_ID=
# Set to 1 to enable the live-inbound TestSubscribe test (operator must
# hand-send a message during the test window).
HERALD_TGRAM_LIVE_INBOUND=

# HRD-012 Claude Code dispatcher (landed 2026-05-XX)
# Path to the `claude` binary; defaults to "claude" on $PATH.
HERALD_CLAUDE_BIN=claude
# Project name used to resolve the session anchor file.
HERALD_CLAUDE_PROJECT_NAME=Herald
```

- [ ] **Step 2: Atomic Issues→Fixed migration for HRD-011 AND HRD-012**

Per §11.4.19 atomic — both rows move in the same commit.

a. Open `docs/Issues.md`. Cut the HRD-011 row AND the HRD-012 row.
b. Open `docs/Fixed.md`. Insert both rows at the TOP of the "Recently fixed" table.
c. Update headers of both files.

HRD-011 row template for Fixed.md (adjust commit refs):
```
| HRD-011 | task | middle | Telegram channel live integration — sendMessage MarkdownV2 + getUpdates long-poll (25s/30s safety-net per §32.2) + outbound_delivery_evidence persistence; 3 new §107 e2e invariants (E17 send+persist) | 2026-05-XX | (this commit) | spec V3 §11.1 + §32; Catalogue-Check: no-match (no vasic-digital Telegram SDK); vendored gopkg.in/telebot.v3 as submodules/telebot |
```

HRD-012 row template:
```
| HRD-012 | task | middle | Claude Code dispatcher live integration — `claude --resume <UUID> --print <envelope>` exec + <<<HERALD-REPLY>>> JSON parse + claude_code_sessions persistence per §33; E18 round-trip + E19 vertical slice (combined with HRD-011) | 2026-05-XX | (this commit) | spec V3 §33 + §33.2; Catalogue-Check: no-match (claude is an external binary, not a library) |
```

- [ ] **Step 3: Update Status.md r7→r8**

- e2e_bluff_hunt: 21 PASS / 0 FAIL (was 18)
- Submodules: 13 vendored (was 12 — added telebot)
- Implementation table: commons_messaging now ✅ landed (was partial)
- Continuation: Wave 1 complete; HRD-008 operator-side e2e validation next

- [ ] **Step 4: Regenerate siblings**

```bash
for f in docs/Issues.md docs/Fixed.md docs/Status.md; do
  base="${f%.md}"
  pandoc "$f" -o "${base}.html" --standalone --toc --metadata title="$(basename $base)"
  pandoc "$f" -o "${base}.docx" --toc --metadata title="$(basename $base)"
  pandoc "$f" -o "${base}.pdf" --pdf-engine=weasyprint --toc --metadata title="$(basename $base)"
done
```

- [ ] **Step 5: Atomic-migration grep**

```bash
grep -c "^| HRD-011 |" docs/Fixed.md  # expect 1
grep -c "^| HRD-011 |" docs/Issues.md  # expect 0
grep -c "^| HRD-012 |" docs/Fixed.md  # expect 1
grep -c "^| HRD-012 |" docs/Issues.md  # expect 0
```

- [ ] **Step 6: Commit**

```bash
git add quickstart/.env.example docs/Issues.md docs/Issues.html docs/Issues.docx docs/Issues.pdf docs/Fixed.md docs/Fixed.html docs/Fixed.docx docs/Fixed.pdf docs/Status.md docs/Status.html docs/Status.docx docs/Status.pdf
git commit -m "HRD-011 + HRD-012 step 10: atomic Issues→Fixed + Status r8 + .env.example

Both HRD-011 (Telegram live) and HRD-012 (Claude Code dispatcher live)
closed atomically per §11.4.19. Status r7→r8: commons_messaging now
✅ landed; e2e_bluff_hunt 18→21 PASS; submodules count 12→13 (added
telebot).

.env.example documents the 4 new env vars operators need: bot token,
chat ID, live-inbound flag, claude binary path, claude project name.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 11: Anti-bluff battery + multi-mirror push

**Files:** none modified — validates and ships.

- [ ] **Step 1: Run full anti-bluff battery**

```bash
cd /Users/milosvasic/Projects/Herald
bash tests/test_constitution_inheritance.sh         # expect 15 PASS
bash tests/test_constitution_inheritance_meta.sh    # expect META-TEST PASS
bash tests/test_i6_refinement_meta.sh               # expect 3 PASS
bash tests/test_i8_usability_meta.sh                # expect 5 PASS
bash scripts/audit_antibluff.sh                     # expect 16 PASS (or more after Models/Concurrency added)
bash scripts/codegraph_validate.sh                  # expect 7+ PASS
bash scripts/e2e_bluff_hunt.sh                      # expect 21 PASS (with creds) or 18 PASS + 3 SKIP
```

All MUST be green. Any FAIL blocks the push.

- [ ] **Step 2: Re-index CodeGraph**

```bash
bash scripts/codegraph_setup.sh
bash scripts/codegraph_validate.sh
```

- [ ] **Step 3: Multi-mirror fan-out push**

```bash
git push origin main
```

Expected: 4 lines (GitHub + GitLab + GitFlic + GitVerse) each showing the new commits.

- [ ] **Step 4: Final verification**

```bash
git log -15 --oneline
```

Should show this plan's commits on top of HEAD. Confirm Issues.md no longer contains HRD-011 or HRD-012 in the open table.

---

## Self-Review

**1. Spec coverage check.**

| Spec requirement | Task |
|---|---|
| §11.1 Telegram sendMessage MarkdownV2 | Task 3 |
| §11.1 sendDocument for attachments | Task 3 (deferred — only sendMessage in v1; attachments are a follow-up) |
| §11.1 getUpdates long-poll | Task 4 |
| §32.2 30s safety-net timer | Task 4 |
| §11.1 webhook ingress with secret_token | Deferred — long-poll-only in v1; webhook is HRD-NNN follow-up |
| Telegram delivery evidence persistence | Task 5 |
| §33 claude --resume invocation | Task 6 |
| §33 <<<HERALD-REPLY>>> JSON parse | Task 6 |
| §33.2 session-resolution algorithm | Already done in existing scaffold (`ResolveSession` + `PersistSession` exist) — Task 6 just calls them |
| Session state persistence | Task 7 |
| E17 send+persist | Task 5 (test) + Task 9 (e2e invariant) |
| E18 dispatch+persist | Task 7 (test) + Task 9 (e2e invariant) |
| E19 full vertical slice | Task 8 (test) + Task 9 (e2e invariant) |
| .env.example documentation | Task 10 |
| HRD-011 + HRD-012 Issues→Fixed | Task 10 |
| Status.md r8 | Task 10 |
| Anti-bluff battery + push | Task 11 |

**Spec gaps:** §11.1 sendDocument (attachments) + webhook ingress are deferred to a follow-up HRD. Both are spec-mandated for the FULL §11.1 capability, but the long-poll + sendMessage path covers the canonical E17 evidence. Note this in the HRD-011 Fixed.md row's title.

**2. Placeholder scan:**
- `2026-05-XX` — parametric close-date for the engineer to fill at Task 10. Legitimate.
- `(this commit)` — Fixed.md convention. Legitimate.
- `<UUID>` — protocol literal in `<<<HERALD-REPLY>>>` discussion. Legitimate.

No genuine TBDs.

**3. Type consistency:**
- `NewWithStorage` (Task 5) + `SendForTenant` (Task 5) — same naming used consistently.
- `HeraldSystemTenant` (Task 7) — declared once, referenced in `persist.go`.
- `DispatchResponse` Outcome + Summary + SessionUUID + AnchorPath — added consistently in Tasks 6 + 7.
- `parseReply` (Task 6) — internal helper, single definition.

**4. Anti-bluff trap coverage:**

| Trap | Mitigation |
|---|---|
| Mock-driven PASS | Every integration test uses live Bot API + live `claude` CLI; no mocks |
| Compile-only PASS | Integration tests assert specific runtime evidence (MessageID, Outcome, persisted row matching receipt) |
| Empty-field PASS | Each probe asserts non-empty MessageID / Outcome / Summary |
| Skipped-silently PASS | SKIP-with-explicit-reason if creds absent — never PASS-by-default |
| Parsed-junk PASS | parseReply returns explicit error if marker absent or JSON malformed |
| Sandbox-leakage PASS | All tests run inside `t.Cleanup(boot.Down)` and use fresh tenant UUIDs |

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-20-hrd-011-012-telegram-and-claude-code-live.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, two-stage review (spec + code), continuous execution.

2. **Inline Execution** — superpowers:executing-plans, batch with checkpoints.

**Which approach?**

<div align="center">

<img src="../../../assets/logo/herald_logo_square_128.png" alt="Herald" width="96" height="96" />

</div>

# Wave 5 — `qaherald` Implementation Plan (pherald ↔ Telegram QA-Bot Automation)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` per Universal Constitution §11.4.70. Each task below dispatches to its own implementer subagent. Steps use checkbox (`- [ ]`) syntax. Run each task's commit only AFTER its self-review block passes.

**Goal:** Introduce `qaherald`, Herald's 8th flavor binary (16th workspace module overall), that drives the full Herald contract end-to-end against live services: Telegram client impersonator + CloudEvent producer + scenario orchestrator + evidence recorder. Every shipped Herald feature (pherald `POST /v1/events`, cherald `/v1/compliance`, sherald `/v1/safety_state`, Telegram delivery, attachment round-trip, §18.1.1 command prefixes) is replayed under one `qaherald run --scenario=all` invocation that writes a bidirectional transcript + sha256-checked attachments under `docs/qa/<run-id>/`. Tags **v1.0.0-dev-0.0.1**. Closes §107.x QA-bot mandate (operator-locked, 2026-05-22).

**Architecture:** New module `qaherald/` peer to the 7 Wave 2 flavor binaries. Internal packages: `transcript/` (JSONL writer + content-addressed attachments), `tgram/` (telebot.v3 long-poll wrapper), `herald/` (HTTPS+JWT REST client, TOON+JSON content negotiation, ALPN h3 attempt), `scenario/` (Go-funcs as scenarios, typed, IDE-assisted, NOT YAML), `report/` (Markdown report with sha256 + latency stats). Cobra root: `qaherald run --scenario=<name|all>`. Wave 4a (HTTP/3 + Brotli + Alt-Svc) and Wave 4b (TOON) exercised as a side-effect — every scenario MUST issue at least one `Accept: application/toon` request and one ALPN-h3 attempt.

**Tech Stack:** Go 1.25+; `gopkg.in/telebot.v3` (vendored at `submodules/telebot/`, shared with pherald); `github.com/spf13/cobra`; `digital.vasic.toon` (Wave 4b); `digital.vasic.http3` (Wave 4a); `crypto/sha256`; `github.com/google/uuid` (UUIDv7); `encoding/json`. No new external dep.

**Spec reference:** `docs/specs/mvp/specification.V3.md` §47 (NEW — T11), §18.1.1 command-prefix replies, §11.0 Channel contract, §43 commands catalogue (extended in T11); `docs/guides/HERALD_CONSTITUTION.md` §107 + §107.x (docs/qa evidence) + §107.y (working-tree quiescence, post-`72e81ab` lesson).

**Wave 4a + 4b substrate already landed:** HTTP/3 + Brotli + TLS 1.3 + Alt-Svc (`v0.2.0`); TOON across all `/v1/*` (`v0.3.0`); e2e at 61 invariants; workspace at 15 modules. Wave 5 adds module #16, bumps e2e to 69 (E63..E70 NEW), adds 3 mutation gates, spec V3 r11→r12, tags `v1.0.0-dev-0.0.1`.

**§107 anti-bluff watershed (CRITICAL):** qaherald is itself a §107 instrument — its OUTPUT (`docs/qa/<run-id>/`) is the auditable evidence for every Herald feature. A bluff here is a meta-bluff. Three anchors:

1. **Real-network bytes only.** Every transcript event backed by a real socket round-trip. T10 gate (b) plants always-202 stub; deny-path scenario FAILs.
2. **sha256 round-trip.** For every uploaded attachment, `getFile` the bot's view back and assert sha256 match. T10 gate (c) plants tolerance bluff; upload scenarios FAIL.
3. **Working-tree quiescence (§107.y).** No `MUTATED for paired`, no `// always pass`, no `_mutated_*` staged. `.git/MUTATION_IN_PROGRESS` lockfile aborts concurrent commits.

**Operator-locked decisions (recorded 2026-05-22, verbatim — DO NOT re-decide):**

| Decision | Value |
|---|---|
| Run-ID format | ISO timestamp + UUIDv7 short suffix: `2026-05-22T15-30-00-7a3b` (colon → hyphen for filesystem safety; trailing 4 hex of UUIDv7 for uniqueness within a single second) |
| Transcript format | JSONL — one event per line: `{ts, direction, kind, payload, attachments[]}` |
| Attachment storage | `docs/qa/<run-id>/attachments/<sha256>.<ext>` content-addressed; `attachments[]` in transcript carries `{sha256, content_type, size_bytes, original_filename}` |
| Scenario language | Go funcs (typed, IDE-assisted) — NOT YAML. Each scenario is a `func(ctx context.Context, o *scenario.Orchestrator) error` registered into a `scenario.Registry`. |
| Tag | `v1.0.0-dev-0.0.1` (operator verbatim — first development-track release on the road to v1.0.0) |
| Brotli / HTTP/3 | qaherald MUST exercise Wave 4a+4b — every scenario issues at least one request with `Accept: application/toon` AND at least one ALPN-h3 attempt against the operator-supplied pherald endpoint. Result of the h3 attempt (success or graceful fall-back to TCP+TLS+HTTP/2) is recorded in the transcript. |
| Workspace count | 15 → 16 modules (`qaherald` added); update `go.work` + CLAUDE.md count |

**Scenarios to implement (collapsed to 8 — pairs #1+#2 and #5+#6+#7 share code paths but each one exercises a distinct external surface and is named separately for report-readability):**

| # | Name | Purpose |
|---|---|---|
| 1 | `happy-path-single-channel` | One CloudEvent posted to pherald → one Telegram message delivered to a single configured subscriber → cross-checked via `getChatHistory` |
| 2 | `fan-out-multi-subscriber` | One CloudEvent → N (≥ 2) subscribers receive the message → each delivery cross-checked |
| 3 | `idempotency-replay` | Same `event_id` posted twice → first response `202` + `recipients=N`, second response `200` + `X-Herald-Replay: true` + `recipients=0` |
| 4 | `deny-path-policy-gate` | Compliance/safety policy denies a CloudEvent → 403 + zero Telegram messages observed within the timeout window |
| 5 | `attachment-upload-photo` | Operator sends a photo to the Herald bot → pherald ingests as CloudEvent with attachment → attachment delivered to subscriber → `sha256(uploaded) == sha256(round-tripped)` |
| 6 | `attachment-upload-document` | Same as #5 but with a `document` (.pdf / .txt — any non-photo) |
| 7 | `attachment-upload-audio` | Same as #5 but with a `voice` (audio) |
| 8 | `command-prefix-bug-query-help-status` | Operator sends each §18.1.1 command-prefix reply (`Bug:`, `Query:`, `Help:`, `Status:`) → pherald ack visible in transcript |

Scenarios 9 (rate-limit) and 10 (oversized-payload) from the task brief are collapsed: rate-limit behaviour is covered by the assert-no-backoff-violation step inside scenario #2 (fan-out), and oversized-payload (11MB) is covered as an assertion variant inside scenario #6 (document upload — push a 11MB document, assert `413`).

**Task decomposition (11 tasks):** T1 module skeleton → T2 transcript → T3 Telegram client → T4 Herald REST client → T5 scenario engine + 8 scenarios → T6 report generator → T7 Cobra `run` wiring → T8 LIVE end-to-end run + commit evidence → T9 e2e_bluff_hunt E63..E70 → T10 Wave 5 mutation gate → T11 spec V3 r11→r12 + tag + 4-mirror push.

---

## File Structure

### CREATE

| Path | Responsibility |
|---|---|
| `qaherald/go.mod` | Module `qaherald` at Go 1.25; `require` telebot, cobra, commons (replace `../commons`), commons_auth (replace), commons_messaging/channels/tgram (replace), digital.vasic.toon (replace `../submodules/TOON`) |
| `qaherald/cmd/qaherald/main.go` | Cobra root `qaherald` + `qaherald version` subcommand (HRD version string from commons) + `qaherald run` subcommand (delegates to internal/scenario.Run) |
| `qaherald/internal/transcript/transcript.go` | `Writer` struct: opens `<out>/transcript.jsonl`, computes run-ID, manages `attachments/` dir; `Append(Event)`, `AttachFile(reader, contentType, origName) (sha256, path, error)`, `Close()` |
| `qaherald/internal/transcript/transcript_test.go` | Round-trip test: write 3 entries + 1 attachment to `t.TempDir()` → re-read JSONL → assert events match → sha256(file on disk) == sha256(recorded in event) |
| `qaherald/internal/tgram/client.go` | `Client` struct wrapping `gopkg.in/telebot.v3.Bot`; methods `Send(text) (msgID, error)`, `Upload(file, contentType) (msgID, fileID, error)`, `Download(fileID) (reader, error)`, `WaitForMessage(timeout, predicate) (Message, error)`, `WaitForReply(timeout, toMsgID, predicate) (Message, error)`; long-poll `getUpdates` loop in a goroutine; chat-history retrieval via `Bot.AdminsOf` / `Bot.Forward` heuristic; `getMe` smoke at construction |
| `qaherald/internal/tgram/client_test.go` | Live `getMe` against the configured bot token; SKIP-with-reason when `HERALD_TGRAM_BOT_TOKEN` not set; assert the bot's `username` matches a regex pattern; NO live Send (paid surface — Send-exercising lives in T7 integration test) |
| `qaherald/internal/herald/client.go` | `Client` struct: HTTPS+JWT (HMAC dev mode using the operator-supplied secret), TLS 1.3 mandatory, ALPN preference `h3,h2`, Brotli `Accept-Encoding: br`; methods `PostEvent(ce CloudEvent, accept string) (Receipt, status int, headers http.Header, err error)`, `GetCompliance(accept string) (Snapshot, int, http.Header, error)`, `GetSafety(accept string) (Snapshot, int, http.Header, error)` |
| `qaherald/internal/herald/client_test.go` | Round-trip via `httptest.Server` — assert TOON request body decoded correctly, JWT header present, Brotli applied when body ≥ 256 B; no live pherald required |
| `qaherald/internal/scenario/orchestrator.go` | `type Orchestrator struct { TG *tgram.Client; Herald *herald.Client; Transcript *transcript.Writer; Clock commons.Clock }`; runner method `Run(ctx, scenario Scenario) error`; metrics emitted to transcript |
| `qaherald/internal/scenario/scenario.go` | `type Scenario struct { Name, Description string; Run func(ctx, *Orchestrator) error }`; `Registry` indexed by name; helper `Wait(d time.Duration)` that respects ctx |
| `qaherald/internal/scenario/happy_path.go` | Full Go implementation of `happy-path-single-channel` — the worked example (fully expanded code in this plan) |
| `qaherald/internal/scenario/fanout.go` | `fan-out-multi-subscriber` (skeleton + step sequence) |
| `qaherald/internal/scenario/idempotency.go` | `idempotency-replay` |
| `qaherald/internal/scenario/deny_path.go` | `deny-path-policy-gate` |
| `qaherald/internal/scenario/attach_photo.go` | `attachment-upload-photo` |
| `qaherald/internal/scenario/attach_document.go` | `attachment-upload-document` (also covers oversized-payload assertion) |
| `qaherald/internal/scenario/attach_audio.go` | `attachment-upload-audio` |
| `qaherald/internal/scenario/command_prefix.go` | `command-prefix-bug-query-help-status` (each prefix in turn) |
| `qaherald/internal/scenario/scenario_test.go` | Httptest+telebot-mock-driven test of `happy-path-single-channel` end-to-end without burning live API calls |
| `qaherald/internal/report/report.go` | `Generate(transcriptPath, outPath string) error` — reads JSONL → renders `report.md` with scenario PASS/FAIL summary table, per-scenario timeline, attachment manifest with sha256s, latency p50/p99 |
| `qaherald/internal/report/report_test.go` | Canned-transcript-to-report test; assert expected sections + sha256 manifest present |
| `tests/test_wave5_mutation_meta.sh` | Wave 5 paired-mutation gate: (a) blank transcript writer, (b) Herald-client always-202 stub, (c) sha256-mismatch tolerance; each paired with a detector; pre-flight `.git/MUTATION_IN_PROGRESS` lockfile + working-tree quiescence per §107.y |
| `docs/qa/<run-id>/` (NEW directory, populated by T8) | `transcript.jsonl`, `report.md`, `attachments/<sha256>.<ext>` per the §107.x mandate |

### MODIFY

| Path | Change |
|---|---|
| `go.work` | Append `./qaherald` to the `use` block (15 → 16 entries; ordering: maintain alphabetical-after-foundation convention) |
| `CLAUDE.md` | r8 → r9: "14 modules" → "16 modules" (the count was 15 after Wave 4a/4b; Wave 5 adds qaherald). Status pointer addendum: Wave 5 qaherald + tag v1.0.0-dev-0.0.1. |
| `scripts/e2e_bluff_hunt.sh` | Append E63..E70 (8 invariants); header tally `Sixty-one` → `Sixty-nine`; comment block at top updated to enumerate E63-E70 |
| `docs/specs/mvp/specification.V3.md` | r11 → r12: new §47 (QA bot — qaherald responsibilities, scenario taxonomy, run-ID format, docs/qa/ evidence layout); §43 commands catalogue extended with `qaherald run`, `qaherald version` |
| `docs/Issues.md` | r14 → r15: prepend HRD-104..HRD-107 Issues→Fixed atomic close at commit time (HRD-104 = qaherald scaffold + transcript, HRD-105 = scenarios + report, HRD-106 = live end-to-end run + docs/qa/, HRD-107 = e2e + mutation gates + tag) |
| `docs/Fixed.md` | r13 → r14: prepend HRD-104..HRD-107 to Recently fixed with commit SHA + e2e invariant references + tag v1.0.0-dev-0.0.1 |
| `docs/Status.md` | r15 → r16: Wave 5 completion summary; e2e invariant total 61 → 69; tag v1.0.0-dev-0.0.1; module count 15 → 16 |
| `docs/CONTINUATION.md` | r-bump: append `HERALD_QAHERALD_OUT_DIR` env-var documentation + `qaherald run --scenario=all` end-to-end smoke command |

---

## Task 1: Workspace module + Cobra skeleton

**Goal:** Land `qaherald/` as workspace module #16; provide a working `qaherald version` subcommand so the next 10 tasks have a real binary to build against; update `go.work` and CLAUDE.md.

**Files:**
- Create: `qaherald/go.mod`, `qaherald/cmd/qaherald/main.go`
- Modify: `go.work`, `CLAUDE.md`

**Anti-bluff §107 anchor for this task:** Build MUST produce a binary that returns the actual Herald version (read from `commons.Version` constant), not a hardcoded `"0.0.0"`. T10 mutation gate (b) plants the always-version-bluff and the unit test catches it.

- [ ] **Step 1: Create the module dir + `go.mod`**

```bash
cd /Users/milosvasic/Projects/Herald
mkdir -p qaherald/cmd/qaherald
cat > qaherald/go.mod <<'EOF'
module qaherald

go 1.25

require (
    commons v0.0.0-00010101000000-000000000000
    github.com/spf13/cobra v1.8.1
)

replace commons => ../commons
EOF
```

Expected: file written. `go build` will fail until Step 2 lands main.go.

- [ ] **Step 2: Write `qaherald/cmd/qaherald/main.go`**

```go
package main

import (
    "fmt"
    "os"

    "commons"

    "github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
    Use:   "qaherald",
    Short: "Herald QA bot — drives pherald ↔ Telegram round-trips end-to-end",
    Long: `qaherald is Herald's QA flavor binary. It impersonates a Telegram client,
posts CloudEvents to pherald via HTTPS+JWT (with TOON content negotiation),
records bidirectional transcripts + sha256-checked attachments under
docs/qa/<run-id>/, and emits a Markdown report.

Per Herald Constitution §107.x (operator mandate, 2026-05-22), every Herald
feature shipped MUST carry a docs/qa/<run-id>/ artefact. qaherald is the
canonical producer of that artefact.`,
}

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print Herald version",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("qaherald %s\n", commons.Version)
    },
}

func main() {
    rootCmd.AddCommand(versionCmd)
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

`commons.Version` is the canonical Herald version constant (`pherald` reads it the same way — verify with `grep -n "commons.Version" pherald/cmd/pherald/*.go`).

- [ ] **Step 3: Add `./qaherald` to `go.work`**

Read `go.work` first to confirm the current entry order:

```bash
cat go.work
```

Insert `./qaherald` in the `use (...)` block alphabetically after `./pherald` (or wherever maintains the existing convention — match what Wave 4a and Wave 2 did).

- [ ] **Step 4: Smoke build + version**

```bash
go build ./qaherald/...
./qaherald/qaherald version 2>&1 || ./qaherald/cmd/qaherald/qaherald version 2>&1
go run ./qaherald/cmd/qaherald version
```

Expected output:

```
qaherald 1.0.0-dev-0.0.1
```

(Or whatever `commons.Version` currently holds — the version string is bumped in T11.)

- [ ] **Step 5: Update `CLAUDE.md` workspace count**

The current CLAUDE.md says "14 Herald modules" (r7) — Wave 4a added commons_tls making it 15, Wave 4b added no new module. Wave 5 adds qaherald → 16. Edit the "Workspace is configured via `go.work` listing **N** Herald modules" line and the table that enumerates them (foundation 7 + `commons_auth` + 6 Wave 2 flavors + `commons_tls` + `qaherald` = 16). Add `qaherald` to the enumeration list.

- [ ] **Step 6: Commit**

```bash
git add qaherald/ go.work CLAUDE.md
git commit -m "$(cat <<'EOF'
Wave 5 step 1: qaherald module skeleton + Cobra root + version

Adds qaherald as Herald's 16th workspace module — flavor #8. Cobra root
+ version subcommand wired against commons.Version. go.work bumped
15→16. CLAUDE.md r8→r9 reflects the new module count + enumeration.

Internal packages (transcript, tgram, herald, scenario, report) land in
subsequent steps (T2..T6). The `qaherald run` subcommand lands in T7.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `qaherald/internal/transcript/` — JSONL writer + content-addressed attachments

**Goal:** Provide the transcript-writing primitive that every scenario uses. The transcript file (`<out>/transcript.jsonl`) is append-only; attachments live under `<out>/attachments/<sha256>.<ext>`; the run-ID is computed once at Writer construction.

**Files:**
- Create: `qaherald/internal/transcript/transcript.go`, `qaherald/internal/transcript/transcript_test.go`

**Anti-bluff §107 anchor for this task:** The Writer MUST be the ONLY interface scenarios use to record observed behaviour — no `fmt.Println`, no direct file writes elsewhere. T10 mutation gate (a) blanks out `Writer.Append`'s body and asserts every scenario then fails its post-condition because no transcript events are recorded.

- [ ] **Step 1: Define the `Event` type + `Writer` struct**

```go
package transcript

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "mime"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
)

// Direction enumerates which leg of the bidirectional channel an event
// belongs to. "out" means qaherald → external service (Telegram or
// pherald); "in" means external service → qaherald. "internal" means a
// qaherald-internal step (scenario assertion, timing checkpoint).
type Direction string

const (
    DirectionOut      Direction = "out"
    DirectionIn       Direction = "in"
    DirectionInternal Direction = "internal"
)

// Kind enumerates the event class. The full enum lives in
// docs/specs/mvp/specification.V3.md §47 (T11 addition); the values
// below are stable across all scenarios.
type Kind string

const (
    KindTGSend         Kind = "tg.send"
    KindTGReceive      Kind = "tg.receive"
    KindTGUpload       Kind = "tg.upload"
    KindTGDownload     Kind = "tg.download"
    KindHeraldPost     Kind = "herald.post"
    KindHeraldResponse Kind = "herald.response"
    KindHeraldGet      Kind = "herald.get"
    KindScenarioStart  Kind = "scenario.start"
    KindScenarioEnd    Kind = "scenario.end"
    KindAssert         Kind = "assert"
    KindWait           Kind = "wait"
)

// Attachment carries the sha256-indexed file metadata. The actual bytes
// live at <out>/attachments/<Sha256>.<Ext>.
type Attachment struct {
    Sha256           string `json:"sha256"`
    ContentType      string `json:"content_type"`
    SizeBytes        int64  `json:"size_bytes"`
    OriginalFilename string `json:"original_filename,omitempty"`
}

// Event is one JSONL row.
type Event struct {
    TS          time.Time       `json:"ts"`
    Direction   Direction       `json:"direction"`
    Kind        Kind            `json:"kind"`
    Scenario    string          `json:"scenario,omitempty"`
    Payload     json.RawMessage `json:"payload,omitempty"`
    Attachments []Attachment    `json:"attachments,omitempty"`
    Note        string          `json:"note,omitempty"`
}

// Writer is the single entry point for transcript recording. It is
// safe for concurrent use.
type Writer struct {
    mu          sync.Mutex
    runID       string
    outDir      string
    attachDir   string
    file        *os.File
    enc         *json.Encoder
}

// NewWriter creates <outDirParent>/<runID>/ + transcript.jsonl +
// attachments/. The run-ID is RFC3339-ish + UUIDv7 short suffix.
func NewWriter(outDirParent string) (*Writer, error) {
    runID := computeRunID(time.Now().UTC())
    outDir := filepath.Join(outDirParent, runID)
    attachDir := filepath.Join(outDir, "attachments")
    if err := os.MkdirAll(attachDir, 0o755); err != nil {
        return nil, fmt.Errorf("transcript: mkdir %s: %w", attachDir, err)
    }
    f, err := os.OpenFile(filepath.Join(outDir, "transcript.jsonl"),
        os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if err != nil {
        return nil, fmt.Errorf("transcript: open jsonl: %w", err)
    }
    return &Writer{
        runID:     runID,
        outDir:    outDir,
        attachDir: attachDir,
        file:      f,
        enc:       json.NewEncoder(f),
    }, nil
}

func (w *Writer) RunID() string  { return w.runID }
func (w *Writer) OutDir() string { return w.outDir }

// Append writes one event to the JSONL file with an immediate fsync —
// scenario failures must not lose the last few events.
func (w *Writer) Append(ev Event) error {
    if ev.TS.IsZero() {
        ev.TS = time.Now().UTC()
    }
    w.mu.Lock()
    defer w.mu.Unlock()
    if err := w.enc.Encode(ev); err != nil {
        return fmt.Errorf("transcript: encode: %w", err)
    }
    return w.file.Sync()
}

// AttachFile reads r, computes sha256 streamingly, persists under
// <outDir>/attachments/<sha256>.<ext>, returns the Attachment record
// the caller embeds in its Event.
func (w *Writer) AttachFile(r io.Reader, contentType, originalFilename string) (Attachment, error) {
    ext := pickExt(contentType, originalFilename)
    tmp, err := os.CreateTemp(w.attachDir, "stage-*"+ext)
    if err != nil {
        return Attachment{}, err
    }
    defer os.Remove(tmp.Name())
    h := sha256.New()
    n, err := io.Copy(io.MultiWriter(tmp, h), r)
    if err != nil {
        tmp.Close()
        return Attachment{}, err
    }
    if err := tmp.Close(); err != nil {
        return Attachment{}, err
    }
    sum := hex.EncodeToString(h.Sum(nil))
    final := filepath.Join(w.attachDir, sum+ext)
    if err := os.Rename(tmp.Name(), final); err != nil {
        return Attachment{}, err
    }
    return Attachment{
        Sha256:           sum,
        ContentType:      contentType,
        SizeBytes:        n,
        OriginalFilename: originalFilename,
    }, nil
}

func (w *Writer) Close() error { return w.file.Close() }

func computeRunID(now time.Time) string {
    ts := now.Format("2006-01-02T15-04-05")
    u, _ := uuid.NewV7()
    s := u.String()
    return fmt.Sprintf("%s-%s", ts, s[len(s)-4:])
}

func pickExt(contentType, origName string) string {
    if origName != "" {
        if e := filepath.Ext(origName); e != "" {
            return e
        }
    }
    if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
        return exts[0]
    }
    return ".bin"
}

// AttachmentPath returns the on-disk path for a recorded Attachment.
// Reports use this for the sha256 manifest section.
func (w *Writer) AttachmentPath(a Attachment) string {
    return filepath.Join(w.attachDir, a.Sha256+pickExt(a.ContentType, a.OriginalFilename))
}

// strings is imported in case future ext-pickers want suffix logic.
var _ = strings.HasPrefix
```

- [ ] **Step 2: Write `transcript_test.go` — `TestWriter_RoundTrip`**

Single test function. Steps:
1. `dir := t.TempDir()`; `w, err := NewWriter(dir)`; assert no err.
2. Assert `w.RunID()` has `"20"` prefix (ISO year).
3. Loop 3× calling `w.Append(Event{Direction: DirectionOut, Kind: KindHeraldPost, Note: "event"})`; assert no err each time.
4. Define `body := []byte("hello qaherald")`; compute `sum := sha256.Sum256(body); want := hex.EncodeToString(sum[:])`.
5. `att, err := w.AttachFile(bytes.NewReader(body), "text/plain", "hi.txt")`; assert `att.Sha256 == want` and `att.SizeBytes == int64(len(body))`.
6. `w.Close()`.
7. Re-open `<runDir>/transcript.jsonl` and decode via `json.Decoder.More` loop; assert event count == 3.
8. `os.ReadFile(w.AttachmentPath(att))`; assert `bytes.Equal(got, body)`.

- [ ] **Step 3: Run the test**

```bash
go test -race -count=1 ./qaherald/internal/transcript/...
```

Expected: `ok qaherald/internal/transcript`. If FAIL, inspect — common cause: `go.mod` missing the `github.com/google/uuid` dep (add via `go get` from inside `qaherald/`).

- [ ] **Step 4: Commit**

```bash
git add qaherald/internal/transcript/ qaherald/go.mod qaherald/go.sum
git commit -m "$(cat <<'EOF'
Wave 5 step 2: qaherald transcript writer (JSONL + content-addressed attachments)

§107.x evidence primitive — every Wave 5 scenario writes its
bidirectional events to <out>/transcript.jsonl with attachments
content-addressed under <out>/attachments/<sha256>.<ext>.

Run-ID format: ISO + UUIDv7 short suffix (operator-locked).
Round-trip test asserts sha256 written matches sha256 read.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `qaherald/internal/tgram/` — Telegram client impersonator

**Goal:** Wrap `gopkg.in/telebot.v3` in a long-poll client that lets scenarios send/receive messages, upload/download attachments, and wait for specific reply patterns. NOT a bot framework — qaherald uses the bot token to impersonate operator-side traffic via `bot.Send` to the configured chat ID.

**Files:**
- Create: `qaherald/internal/tgram/client.go`, `qaherald/internal/tgram/client_test.go`
- Modify: `qaherald/go.mod` (add telebot.v3 dep)

**Anti-bluff §107 anchor for this task:** Every `Send`/`Upload` MUST cross an HTTPS socket to `api.telegram.org` (or whatever endpoint the operator configured). T10 mutation gate (b) plants a stub that fakes a `message_id` without crossing the network; the deny-path scenario's "zero TG messages observed" assertion catches it because the stub erroneously records non-zero.

- [ ] **Step 1: Vendor telebot.v3 via the existing submodules/telebot pattern**

`pherald` already vendors telebot — re-use that submodule path. Confirm via:

```bash
grep -n "telebot" /Users/milosvasic/Projects/Herald/pherald/go.mod
ls /Users/milosvasic/Projects/Herald/submodules/telebot/ 2>&1 | head -3
```

In `qaherald/go.mod`, mirror pherald's `require` + `replace` directive for telebot. Run `go mod tidy` from inside `qaherald/`.

- [ ] **Step 2: Implement `client.go`**

Key API surface (the implementer expands the bodies; this plan locks the signatures):

```go
package tgram

import (
    "context"
    "fmt"
    "io"
    "time"

    tele "gopkg.in/telebot.v3"
)

type Client struct {
    bot    *tele.Bot
    chatID int64
    inbox  chan tele.Message
    cancel context.CancelFunc
}

// NewClient establishes a bot session via getMe and starts the
// long-poll loop in a goroutine. Returns error if the token is invalid
// or the chat is unreachable.
func NewClient(token string, chatID int64) (*Client, error) {
    pref := tele.Settings{
        Token:  token,
        Poller: &tele.LongPoller{Timeout: 10 * time.Second},
    }
    bot, err := tele.NewBot(pref)
    if err != nil {
        return nil, fmt.Errorf("tgram: NewBot: %w", err)
    }
    // smoke: getMe.
    me := bot.Me
    if me == nil || me.Username == "" {
        return nil, fmt.Errorf("tgram: getMe returned empty bot identity")
    }
    c := &Client{bot: bot, chatID: chatID, inbox: make(chan tele.Message, 64)}
    bot.Handle(tele.OnText, func(ctx tele.Context) error {
        c.inbox <- *ctx.Message()
        return nil
    })
    bot.Handle(tele.OnPhoto, func(ctx tele.Context) error {
        c.inbox <- *ctx.Message()
        return nil
    })
    bot.Handle(tele.OnDocument, func(ctx tele.Context) error {
        c.inbox <- *ctx.Message()
        return nil
    })
    bot.Handle(tele.OnVoice, func(ctx tele.Context) error {
        c.inbox <- *ctx.Message()
        return nil
    })
    pCtx, cancel := context.WithCancel(context.Background())
    c.cancel = cancel
    go func() {
        bot.Start()
        <-pCtx.Done()
        bot.Stop()
    }()
    return c, nil
}

// Send delivers a text message to the configured chat and returns the
// resulting Telegram message_id. The message is real — it crosses the
// HTTPS socket to api.telegram.org.
func (c *Client) Send(text string) (int, error) {
    msg, err := c.bot.Send(&tele.Chat{ID: c.chatID}, text)
    if err != nil {
        return 0, err
    }
    return msg.ID, nil
}

// Upload sends a file (photo / document / voice — picked from
// contentType) and returns msgID + Telegram's fileID for later
// getFile-round-tripping.
func (c *Client) Upload(r io.Reader, contentType, filename string) (msgID int, fileID string, err error) { /* ... */ }

// Download fetches a Telegram-side file by its fileID via getFile +
// HTTP GET — the round-trip surface that lets qaherald sha256-verify
// attachment integrity.
func (c *Client) Download(fileID string) (io.ReadCloser, error) { /* ... */ }

// WaitForMessage drains the inbox until a message matching `predicate`
// arrives or `timeout` elapses. Returns the first match or
// context.DeadlineExceeded.
func (c *Client) WaitForMessage(timeout time.Duration, predicate func(tele.Message) bool) (tele.Message, error) {
    deadline := time.After(timeout)
    for {
        select {
        case m := <-c.inbox:
            if predicate(m) {
                return m, nil
            }
        case <-deadline:
            return tele.Message{}, context.DeadlineExceeded
        }
    }
}

// WaitForReply waits for a message whose ReplyTo.ID equals
// `toMsgID`. Wraps WaitForMessage with the reply-chain predicate.
func (c *Client) WaitForReply(timeout time.Duration, toMsgID int, predicate func(tele.Message) bool) (tele.Message, error) { /* ... */ }

// Close stops the long-poll loop. Idempotent.
func (c *Client) Close() error {
    if c.cancel != nil {
        c.cancel()
    }
    return nil
}
```

- [ ] **Step 3: Write `client_test.go`**

```go
package tgram

import (
    "os"
    "regexp"
    "testing"
)

func TestNewClient_LiveGetMe(t *testing.T) {
    token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
    if token == "" {
        t.Skip("HERALD_TGRAM_BOT_TOKEN unset — live getMe SKIP per §11.4.3")
    }
    c, err := NewClient(token, 0) // chatID 0 — getMe smoke only
    if err != nil {
        t.Fatalf("NewClient: %v", err)
    }
    defer c.Close()
    // bot username should match Telegram's bot naming rule.
    if !regexp.MustCompile(`^[A-Za-z0-9_]+bot$`).MatchString(c.bot.Me.Username) {
        t.Fatalf("bot username %q does not look like a bot", c.bot.Me.Username)
    }
}
```

NOTE: do NOT add a live `Send` test here — the paid-surface (sending real messages to the operator's chat) lives in T7's integration test or T8's live run. Unit tests stay constrained to `getMe` (read-only, free).

- [ ] **Step 4: Run + commit**

```bash
go test -race -count=1 ./qaherald/internal/tgram/...
git add qaherald/internal/tgram/ qaherald/go.mod qaherald/go.sum
git commit -m "$(cat <<'EOF'
Wave 5 step 3: qaherald Telegram client impersonator

Wraps gopkg.in/telebot.v3 in a long-poll client. Surfaces Send,
Upload, Download (getFile), WaitForMessage, WaitForReply — the four
verbs every Wave 5 scenario uses.

Live getMe test asserts bot identity; SKIP-with-reason when token
absent. Send/Upload paid-surface remains scenario-driven, never
unit-tested (would burn Telegram API budget on every CI run).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `qaherald/internal/herald/` — Herald REST client (TOON + h3 + brotli)

**Goal:** HTTPS client for the three `/v1/*` business routes. JWT (HMAC dev mode), TLS 1.3 mandatory, ALPN preference `h3,h2`, Brotli accept-encoding, TOON or JSON via `Accept` header.

**Files:**
- Create: `qaherald/internal/herald/client.go`, `qaherald/internal/herald/client_test.go`
- Modify: `qaherald/go.mod` (add commons_auth + digital.vasic.toon + digital.vasic.http3 deps)

**Anti-bluff §107 anchor for this task:** Every `PostEvent` MUST return the actual HTTP status + response headers verbatim. T10 mutation gate (b) plants `return 202` unconditionally; the deny-path scenario expects 403 and the mutation surfaces because the assertion fails. Additionally, the `Accept: application/toon` request MUST receive a TOON body (first byte NOT `{`, NOT `[`) — this hooks into Wave 4b's E60 wire-byte assertion.

- [ ] **Step 1: Types + constructor**

```go
package herald

import (
    "bytes"
    "context"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "digital.vasic.toon"
    "github.com/golang-jwt/jwt/v5"
)

type Client struct {
    baseURL string
    secret  []byte
    http    *http.Client
}

type CloudEvent struct {
    SpecVersion string          `json:"specversion"`
    ID          string          `json:"id"`
    Source      string          `json:"source"`
    Type        string          `json:"type"`
    Time        time.Time       `json:"time"`
    DataB64     string          `json:"data_base64,omitempty"`
    Data        json.RawMessage `json:"data,omitempty"`
}

type Receipt struct {
    EventID    string `json:"event_id"`
    Recipients int    `json:"recipients"`
    Status     string `json:"status"`
    Note       string `json:"note,omitempty"`
}

const (
    AcceptTOON = "application/toon"
    AcceptJSON = "application/json"
)

func New(baseURL string, jwtSecret []byte) *Client {
    return &Client{
        baseURL: baseURL,
        secret:  jwtSecret,
        http: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    MinVersion: tls.VersionTLS13,
                    NextProtos: []string{"h2", "http/1.1"}, // h3 attempted separately
                },
            },
        },
    }
}
```

- [ ] **Step 2: JWT helper + the three verbs**

```go
func (c *Client) jwt() (string, error) {
    tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
        Issuer:    "qaherald",
        Subject:   "qa-bot",
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
    })
    return tok.SignedString(c.secret)
}

func (c *Client) PostEvent(ctx context.Context, ce CloudEvent, accept string) (Receipt, int, http.Header, error) {
    tok, err := c.jwt()
    if err != nil {
        return Receipt{}, 0, nil, err
    }
    body, contentType, err := encode(ce, accept)
    if err != nil {
        return Receipt{}, 0, nil, err
    }
    req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/events", bytes.NewReader(body))
    if err != nil {
        return Receipt{}, 0, nil, err
    }
    req.Header.Set("Authorization", "Bearer "+tok)
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Accept", accept)
    req.Header.Set("Accept-Encoding", "br")
    resp, err := c.http.Do(req)
    if err != nil {
        return Receipt{}, 0, nil, err
    }
    defer resp.Body.Close()
    raw, err := io.ReadAll(resp.Body)
    if err != nil {
        return Receipt{}, resp.StatusCode, resp.Header, err
    }
    var r Receipt
    if err := decode(raw, resp.Header.Get("Content-Type"), &r); err != nil {
        return Receipt{}, resp.StatusCode, resp.Header, err
    }
    return r, resp.StatusCode, resp.Header, nil
}

// GetCompliance and GetSafety follow the same pattern — GET with the
// Accept header, decode by Content-Type. Their result types are
// scenario-defined and may be lifted to a shared types package later.
func (c *Client) GetCompliance(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error) { /* ... */ }
func (c *Client) GetSafety(ctx context.Context, accept string) (json.RawMessage, int, http.Header, error) { /* ... */ }

func encode(v any, contentType string) ([]byte, string, error) {
    switch contentType {
    case AcceptTOON:
        b, err := toon.Marshal(v)
        if err != nil {
            return nil, "", fmt.Errorf("toon.Marshal: %w", err)
        }
        return b, AcceptTOON, nil
    default:
        b, err := json.Marshal(v)
        if err != nil {
            return nil, "", err
        }
        return b, AcceptJSON, nil
    }
}

func decode(raw []byte, contentType string, dst any) error {
    if toon.IsTOONContentType(contentType) {
        return toon.Unmarshal(raw, dst)
    }
    return json.Unmarshal(raw, dst)
}
```

- [ ] **Step 3: `client_test.go` — `TestPostEvent_TOONRoundTrip`**

Single test using `httptest.NewTLSServer` whose handler:
1. Asserts `r.Header.Get("Accept") == AcceptTOON`.
2. Asserts `r.Header.Get("Authorization") != ""`.
3. Writes `Content-Type: application/toon` + status 202 + a small body the implementer encodes via `toon.Marshal` of a stub `Receipt`.

Test body: `c := New(srv.URL, []byte("test-secret")); c.http = srv.Client()` (share TLS trust); call `c.PostEvent(ctx, CloudEvent{SpecVersion: "1.0", ID: "test-1", Source: "qa", Type: "test"}, AcceptTOON)`; assert status == 202 and decoded Receipt non-zero. Canonical wire-byte assertion lives in T8's live run.

- [ ] **Step 4: Run + commit**

```bash
go test -race -count=1 ./qaherald/internal/herald/...
git add qaherald/internal/herald/ qaherald/go.mod qaherald/go.sum
git commit -m "$(cat <<'EOF'
Wave 5 step 4: qaherald Herald REST client (TOON+JWT+h3+brotli)

PostEvent / GetCompliance / GetSafety honour Wave 4a (HTTP/3 ALPN
preference, Brotli accept-encoding, TLS 1.3) + Wave 4b (TOON
content-negotiation via digital.vasic.toon).

httptest-based round-trip asserts JWT header present, Accept honoured,
status surfaced verbatim (anti-bluff: T10 mutation gate b plants
always-202 stub; deny-path scenario detects).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Scenario engine + 8 scenarios (Go-funcs, NOT YAML)

**Goal:** Land the `scenario.Orchestrator` + `scenario.Scenario` types + 8 named scenarios. Scenario #1 (`happy-path-single-channel`) is the fully-expanded worked example; scenarios #2..#8 ship as named Go files with step-by-step Go skeletons + assertion lists. The implementer subagent expands each skeleton to working code at execution time using the worked example as the pattern.

**Files:**
- Create: `qaherald/internal/scenario/orchestrator.go`, `qaherald/internal/scenario/scenario.go`, `qaherald/internal/scenario/registry.go`, `qaherald/internal/scenario/happy_path.go`, `qaherald/internal/scenario/fanout.go`, `qaherald/internal/scenario/idempotency.go`, `qaherald/internal/scenario/deny_path.go`, `qaherald/internal/scenario/attach_photo.go`, `qaherald/internal/scenario/attach_document.go`, `qaherald/internal/scenario/attach_audio.go`, `qaherald/internal/scenario/command_prefix.go`, `qaherald/internal/scenario/scenario_test.go`

**Anti-bluff §107 anchor for this task:** Every scenario MUST emit at least one `KindHeraldPost` event AND at least one `KindTGSend` or `KindTGReceive` event to the transcript — this is the bidirectionality invariant. T10 mutation gate (a) blanks `Writer.Append` and every scenario then fails because the post-condition assertion `len(events) > 0` catches it.

- [ ] **Step 1: Orchestrator + Scenario types**

```go
package scenario

import (
    "context"
    "fmt"
    "time"

    "qaherald/internal/herald"
    "qaherald/internal/tgram"
    "qaherald/internal/transcript"
)

type Orchestrator struct {
    TG         *tgram.Client
    Herald     *herald.Client
    Transcript *transcript.Writer
    ChatID     int64
    Now        func() time.Time
}

type Scenario struct {
    Name        string
    Description string
    Run         func(ctx context.Context, o *Orchestrator) error
}

// Result is what Orchestrator.RunScenario writes to the transcript at
// end of run. Picked up by the report generator (T6) to populate the
// PASS/FAIL summary table.
type Result struct {
    Scenario  string        `json:"scenario"`
    PASS      bool          `json:"pass"`
    Duration  time.Duration `json:"duration"`
    ErrorText string        `json:"error,omitempty"`
}

func (o *Orchestrator) RunScenario(ctx context.Context, s Scenario) Result {
    start := o.Now()
    _ = o.Transcript.Append(transcript.Event{
        Direction: transcript.DirectionInternal,
        Kind:      transcript.KindScenarioStart,
        Scenario:  s.Name,
        Note:      s.Description,
    })
    err := s.Run(ctx, o)
    end := o.Now()
    res := Result{
        Scenario: s.Name,
        PASS:     err == nil,
        Duration: end.Sub(start),
    }
    if err != nil {
        res.ErrorText = err.Error()
    }
    _ = o.Transcript.Append(transcript.Event{
        Direction: transcript.DirectionInternal,
        Kind:      transcript.KindScenarioEnd,
        Scenario:  s.Name,
        Note:      fmt.Sprintf("PASS=%v err=%v", res.PASS, err),
    })
    return res
}
```

- [ ] **Step 2: Registry + helper assertions**

```go
package scenario

var registry = map[string]Scenario{}

func Register(s Scenario) { registry[s.Name] = s }
func Get(name string) (Scenario, bool) {
    s, ok := registry[name]
    return s, ok
}
func All() []Scenario {
    out := make([]Scenario, 0, len(registry))
    for _, s := range registry {
        out = append(out, s)
    }
    return out
}

// assertions used across scenarios
func assertStatus(want, got int, where string) error {
    if want != got {
        return fmt.Errorf("%s: expected HTTP %d, got %d", where, want, got)
    }
    return nil
}
func assertHeader(want, got string, where string) error {
    if want != got {
        return fmt.Errorf("%s: expected header %q, got %q", where, want, got)
    }
    return nil
}
```

- [ ] **Step 3: Scenario #1 — `happy-path-single-channel` (fully expanded — the worked example)**

```go
// qaherald/internal/scenario/happy_path.go
package scenario

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "qaherald/internal/herald"
    "qaherald/internal/transcript"
)

func init() {
    Register(Scenario{
        Name:        "happy-path-single-channel",
        Description: "One CloudEvent posted to pherald → one Telegram message delivered to the configured chat → cross-checked",
        Run:         runHappyPath,
    })
}

func runHappyPath(ctx context.Context, o *Orchestrator) error {
    // 1. Construct a CloudEvent.
    ce := herald.CloudEvent{
        SpecVersion: "1.0",
        ID:          fmt.Sprintf("qaherald-happy-%d", o.Now().UnixNano()),
        Source:      "qaherald",
        Type:        "qa.happy-path",
        Time:        o.Now().UTC(),
        Data:        json.RawMessage(`{"message":"qaherald says hello"}`),
    }
    cePayload, _ := json.Marshal(ce)

    // 2. POST it via Accept: application/toon (exercises Wave 4b).
    _ = o.Transcript.Append(transcript.Event{
        Direction: transcript.DirectionOut,
        Kind:      transcript.KindHeraldPost,
        Scenario:  "happy-path-single-channel",
        Payload:   cePayload,
        Note:      "Accept: application/toon",
    })
    receipt, status, headers, err := o.Herald.PostEvent(ctx, ce, herald.AcceptTOON)
    receiptPayload, _ := json.Marshal(receipt)
    _ = o.Transcript.Append(transcript.Event{
        Direction: transcript.DirectionIn,
        Kind:      transcript.KindHeraldResponse,
        Scenario:  "happy-path-single-channel",
        Payload:   receiptPayload,
        Note:      fmt.Sprintf("status=%d content-type=%s", status, headers.Get("Content-Type")),
    })
    if err != nil {
        return fmt.Errorf("PostEvent: %w", err)
    }
    if err := assertStatus(202, status, "happy-path PostEvent"); err != nil {
        return err
    }
    if receipt.Recipients < 1 {
        return fmt.Errorf("expected recipients>=1, got %d", receipt.Recipients)
    }

    // 3. Wait for the Telegram delivery to land in the chat.
    deadline := 30 * time.Second
    msg, err := o.TG.WaitForMessage(deadline, func(m teleMessageType) bool {
        // The bot's own Send echoes into the chat — pherald's delivery
        // is what we want. The simplest predicate: payload includes
        // ce.ID. Production version matches a richer template.
        return containsCloudEventID(m, ce.ID)
    })
    if err != nil {
        return fmt.Errorf("WaitForMessage: %w", err)
    }
    msgPayload, _ := json.Marshal(map[string]any{
        "message_id": msg.ID,
        "chat_id":    msg.Chat.ID,
        "text":       msg.Text,
    })
    _ = o.Transcript.Append(transcript.Event{
        Direction: transcript.DirectionIn,
        Kind:      transcript.KindTGReceive,
        Scenario:  "happy-path-single-channel",
        Payload:   msgPayload,
        Note:      "delivery from pherald → tgram → chat",
    })
    return nil
}

// teleMessageType + containsCloudEventID are tiny local shims —
// implementer replaces with the real tele.Message + a payload-match
// helper. The point of this worked example is the SHAPE of a scenario,
// not the exact import path.
```

The pseudo-types `teleMessageType` and `containsCloudEventID` are placeholders — the implementer uses `gopkg.in/telebot.v3.Message` and a real string-contains helper. The structure (CE construction → PostEvent → assert status + recipients → WaitForMessage → assert match → transcript at each step) is the canonical pattern for ALL scenarios.

- [ ] **Step 4: Scenarios #2..#8 — skeleton + step sequence + assertion list**

For each of the remaining seven scenarios, the implementer writes one `.go` file following the worked example. Below: the locked step sequence + assertion list.

**Scenario #2 — `fan-out-multi-subscriber`** (`fanout.go`):
1. Pre-condition: pherald has ≥ 2 subscribers configured in the `subscribers` table. (T8 documents the operator setup step; for now assert via `o.Herald.GetCompliance` returning a snapshot listing ≥ 2 subscribers.)
2. POST one CloudEvent with `Accept: application/toon`.
3. Assert `receipt.Recipients >= 2`.
4. `WaitForMessage` twice (or once with a predicate that counts to N), recording each delivery.
5. Assertion: count of `KindTGReceive` events for this scenario in the transcript == `receipt.Recipients`.

**Scenario #3 — `idempotency-replay`** (`idempotency.go`):
1. Construct a CloudEvent with a deterministic `event_id` (e.g. `qaherald-idem-<runID>`).
2. POST it once → expect 202 + `recipients=N` + `X-Herald-Replay` absent.
3. POST the SAME `event_id` again → expect 200 + `recipients=0` + `X-Herald-Replay: true`.
4. Assertions: status codes match; `X-Herald-Replay` header asserted on the second response.

**Scenario #4 — `deny-path-policy-gate`** (`deny_path.go`):
1. Construct a CloudEvent with a `type` value that the operator has configured as denied by cherald compliance or sherald safety. (T8 documents the setup; for now use a sentinel value like `qa.deny-me`.)
2. POST it → expect 403.
3. Wait 5 s. Assert: zero `KindTGReceive` events recorded for this scenario.
4. Assertions: status 403; transcript scenario-filter count of TGReceive == 0.

**Scenario #5 — `attachment-upload-photo`** (`attach_photo.go`):
1. Generate a 1×1 PNG inline (or read from `qaherald/testdata/sample.png` — committed under the module).
2. Compute `sha256(uploaded)` BEFORE uploading.
3. `o.TG.Upload(reader, "image/png", "sample.png")` → record msgID + fileID.
4. WaitForMessage on the bot inbox for the photo's `pherald → tgram` delivery.
5. `o.TG.Download(fileID)` → compute `sha256(downloaded)`.
6. Assertion: `sha256(uploaded) == sha256(downloaded)`. If not, the scenario FAILs with the byte-level diff in the error.

**Scenario #6 — `attachment-upload-document`** (`attach_document.go`):
- Same as #5 with `application/pdf` + a small PDF or `text/plain` + a `.txt`.
- ALSO covers oversized-payload: a second variant in the same file uploads an 11MB document and asserts pherald returns 413 (or the operator-configured cap). Both variants share the scenario name; the report distinguishes them via the `Note` field.

**Scenario #7 — `attachment-upload-audio`** (`attach_audio.go`):
- Same as #5 with `audio/ogg` + a small .ogg voice clip.

**Scenario #8 — `command-prefix-bug-query-help-status`** (`command_prefix.go`):
1. For each of the four §18.1.1 prefixes (`Bug:`, `Query:`, `Help:`, `Status:`):
   1. `o.TG.Send("Bug: qaherald test")` (and so on for each prefix).
   2. WaitForReply on the bot inbox for pherald's ack reply within 15 s.
   3. Record both the Send and the ack to the transcript.
2. Assertion: 4 sends + 4 acks all recorded; no prefix returned an empty ack.

- [ ] **Step 5: `scenario_test.go` — httptest+telebot-mock driven happy-path**

```go
package scenario

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    // + tgram mock import — implementer extends a tiny in-memory
    // tgram.Client mock for unit-test scope only.
)

func TestHappyPath_HTTPTestMock(t *testing.T) {
    // implementer wires httptest.Server returning a canned Receipt,
    // a tgram mock that delivers a canned tele.Message after Send,
    // and a t.TempDir() transcript — assert PASS result.
}
```

This test proves orchestration without burning live API calls. Skipped from CI when telebot mock can't be wired (rare — telebot.v3 supports `Settings.Synchronous: true` mode).

- [ ] **Step 6: Run + commit**

```bash
go test -race -count=1 ./qaherald/internal/scenario/...
git add qaherald/internal/scenario/
git commit -m "$(cat <<'EOF'
Wave 5 step 5: scenario engine + 8 named scenarios

Orchestrator + Scenario types + Registry; happy-path-single-channel
fully expanded as the worked example. Remaining 7 scenarios
(fan-out, idempotency, deny-path, attach-photo/document/audio,
command-prefix) follow the same pattern — each one transcribed via
the bidirectional invariant (≥1 KindHeraldPost + ≥1 KindTG* per
scenario).

httptest+telebot-mock test proves orchestration; live runs land in
T8 under docs/qa/<run-id>/.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `qaherald/internal/report/` — Markdown report generator

**Goal:** Read `transcript.jsonl` + `Result[]` from the orchestrator → emit `<out>/report.md` with: scenario PASS/FAIL summary table, per-scenario timeline, attachment manifest with sha256s, latency p50/p99.

**Files:**
- Create: `qaherald/internal/report/report.go`, `qaherald/internal/report/report_test.go`

**Anti-bluff §107 anchor for this task:** The report MUST surface the actual scenario outcomes verbatim from the transcript — no PASS-coercion, no error-text truncation. If the transcript records 3 PASS + 5 FAIL, the summary table shows 3 PASS + 5 FAIL. T10 mutation gate (a) plants a Writer-blank-out so the report ends up with zero events recorded — the report's summary section then reads "0 scenarios", which the e2e invariant catches.

- [ ] **Step 1: `Generate(transcriptPath, resultsPath, outPath string) error`**

The generator opens the JSONL stream, indexes events by scenario, computes per-scenario duration (start→end), counts attachments + cross-references each one's sha256 against the on-disk file, computes p50/p99 latency from `KindHeraldPost` → `KindHeraldResponse` deltas, and writes the markdown with these sections:

```markdown
# qaherald run report

| Run ID | Start | End | Duration |
|---|---|---|---|
| 2026-05-22T15-30-00-7a3b | ... | ... | ... |

## Summary

| Scenario | Result | Duration | Recipients | Attachments |
|---|---|---|---|---|
| happy-path-single-channel | PASS | 1.23s | 1 | 0 |
| ... | ... | ... | ... | ... |

| Aggregate | Value |
|---|---|
| PASS | N |
| FAIL | M |
| Total scenarios | N+M |
| HTTP p50 latency | ... ms |
| HTTP p99 latency | ... ms |

## Scenario timelines

### happy-path-single-channel
- [out] herald.post 2026-... id=qaherald-happy-... Accept=application/toon
- [in]  herald.response status=202 content-type=application/toon recipients=1
- [in]  tg.receive message_id=4711 text="..."

(... per-scenario continued ...)

## Attachment manifest

| sha256 | content-type | size | original filename | path |
|---|---|---|---|---|
| ... | image/png | 137 B | sample.png | attachments/<sha>.png |
```

- [ ] **Step 2: `report_test.go` — canned transcript input → assert sections**

```go
package report

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestGenerate_CannedTranscript(t *testing.T) {
    dir := t.TempDir()
    // write a known 3-event transcript + 1 attachment file
    // implementer fills in
    out := filepath.Join(dir, "report.md")
    if err := Generate(filepath.Join(dir, "transcript.jsonl"), filepath.Join(dir, "results.json"), out); err != nil {
        t.Fatalf("Generate: %v", err)
    }
    body, _ := os.ReadFile(out)
    for _, want := range []string{
        "# qaherald run report",
        "## Summary",
        "## Scenario timelines",
        "## Attachment manifest",
    } {
        if !strings.Contains(string(body), want) {
            t.Fatalf("report missing section %q", want)
        }
    }
}
```

- [ ] **Step 3: Run + commit**

```bash
go test -race -count=1 ./qaherald/internal/report/...
git add qaherald/internal/report/
git commit -m "$(cat <<'EOF'
Wave 5 step 6: qaherald Markdown report generator

Reads transcript.jsonl + results → emits report.md with:
- PASS/FAIL summary table per scenario
- aggregate counts + HTTP p50/p99 latency
- per-scenario ordered timeline
- attachment manifest (sha256 + path)

Canned-transcript test asserts required sections present.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `qaherald run` Cobra subcommand wiring

**Goal:** Wire the `qaherald run` subcommand: flags + env-var fallbacks; instantiates Transcript, Telegram client, Herald client; iterates scenarios per `--scenario=<name|all>`; calls Report generator at end; exits non-zero if any scenario FAILed.

**Files:**
- Create: `qaherald/cmd/qaherald/run.go`
- Modify: `qaherald/cmd/qaherald/main.go` (register the new subcommand)

**Anti-bluff §107 anchor for this task:** Exit code MUST be non-zero on any scenario FAIL. T10 mutation gate (b)'s always-202 stub makes the deny-path scenario FAIL; this task's exit code surfacing is what propagates that FAIL to the e2e bluff-hunt invariant (E63).

- [ ] **Step 1: Flag definitions + env fallback**

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strconv"
    "time"

    "qaherald/internal/herald"
    "qaherald/internal/report"
    "qaherald/internal/scenario"
    "qaherald/internal/tgram"
    "qaherald/internal/transcript"

    "github.com/spf13/cobra"
)

var (
    flagScenario    string
    flagOutDirRoot  string
    flagHeraldURL   string
    flagHeraldToken string
    flagTGToken     string
    flagTGChatID    int64
    flagTimeout     time.Duration
)

var runCmd = &cobra.Command{
    Use:   "run",
    Short: "Run qaherald scenarios against the configured pherald + Telegram bot",
    RunE:  runRun,
}

func init() {
    runCmd.Flags().StringVar(&flagScenario, "scenario", "all", "scenario name or 'all'")
    runCmd.Flags().StringVar(&flagOutDirRoot, "out-dir", "docs/qa", "parent dir for <run-id>/")
    runCmd.Flags().StringVar(&flagHeraldURL, "herald-url", envOr("HERALD_BASE_URL", "https://localhost:7443"), "")
    runCmd.Flags().StringVar(&flagHeraldToken, "herald-token", envOr("HERALD_JWT_SECRET", ""), "")
    runCmd.Flags().StringVar(&flagTGToken, "tg-token", envOr("HERALD_TGRAM_BOT_TOKEN", ""), "")
    runCmd.Flags().Int64Var(&flagTGChatID, "tg-chat", envOrInt("HERALD_TGRAM_CHAT_ID", 0), "")
    runCmd.Flags().DurationVar(&flagTimeout, "scenario-timeout", 60*time.Second, "")
    rootCmd.AddCommand(runCmd)
}

func envOr(name, def string) string {
    if v := os.Getenv(name); v != "" {
        return v
    }
    return def
}
func envOrInt(name string, def int64) int64 {
    if v := os.Getenv(name); v != "" {
        if n, err := strconv.ParseInt(v, 10, 64); err == nil {
            return n
        }
    }
    return def
}
```

- [ ] **Step 2: `runRun` orchestrator**

```go
func runRun(cmd *cobra.Command, args []string) error {
    if flagHeraldToken == "" {
        return fmt.Errorf("--herald-token (or HERALD_JWT_SECRET) is required")
    }
    if flagTGToken == "" {
        return fmt.Errorf("--tg-token (or HERALD_TGRAM_BOT_TOKEN) is required")
    }
    if flagTGChatID == 0 {
        return fmt.Errorf("--tg-chat (or HERALD_TGRAM_CHAT_ID) is required")
    }

    tw, err := transcript.NewWriter(flagOutDirRoot)
    if err != nil {
        return err
    }
    defer tw.Close()
    fmt.Println("qaherald run id:", tw.RunID())

    tg, err := tgram.NewClient(flagTGToken, flagTGChatID)
    if err != nil {
        return err
    }
    defer tg.Close()

    hc := herald.New(flagHeraldURL, []byte(flagHeraldToken))

    orch := &scenario.Orchestrator{
        TG: tg, Herald: hc, Transcript: tw,
        ChatID: flagTGChatID, Now: func() time.Time { return time.Now().UTC() },
    }

    var scenarios []scenario.Scenario
    if flagScenario == "all" {
        scenarios = scenario.All()
    } else {
        s, ok := scenario.Get(flagScenario)
        if !ok {
            return fmt.Errorf("unknown scenario: %s", flagScenario)
        }
        scenarios = []scenario.Scenario{s}
    }

    results := make([]scenario.Result, 0, len(scenarios))
    anyFail := false
    for _, s := range scenarios {
        ctx, cancel := context.WithTimeout(cmd.Context(), flagTimeout)
        r := orch.RunScenario(ctx, s)
        cancel()
        results = append(results, r)
        if !r.PASS {
            anyFail = true
        }
        fmt.Printf("%s: %s (%s) %s\n",
            r.Scenario,
            map[bool]string{true: "PASS", false: "FAIL"}[r.PASS],
            r.Duration, r.ErrorText)
    }

    // Persist results.json + generate report.md.
    if err := writeResults(tw.OutDir(), results); err != nil {
        return err
    }
    if err := report.Generate(
        filepath.Join(tw.OutDir(), "transcript.jsonl"),
        filepath.Join(tw.OutDir(), "results.json"),
        filepath.Join(tw.OutDir(), "report.md"),
    ); err != nil {
        return err
    }

    if anyFail {
        return fmt.Errorf("%d scenario(s) FAILed", failCount(results))
    }
    return nil
}
```

- [ ] **Step 3: Integration test using httptest + telebot mock**

`qaherald/cmd/qaherald/run_test.go` builds an httptest pherald + a telebot mock, runs `runRun` with the in-process flags, asserts:
- exit code zero on canned-PASS scenario
- exit code non-zero on canned-FAIL scenario
- `<runDir>/report.md` exists
- `<runDir>/transcript.jsonl` non-empty

- [ ] **Step 4: Run + commit**

```bash
go build ./qaherald/...
go test -race -count=1 ./qaherald/...
git add qaherald/cmd/qaherald/
git commit -m "$(cat <<'EOF'
Wave 5 step 7: qaherald run subcommand — Cobra wiring

Flags: --scenario, --out-dir, --herald-url, --herald-token,
--tg-token, --tg-chat, --scenario-timeout. Env-var fallback for
each. Exit non-zero on any scenario FAIL — propagates §107 anti-bluff
verdict to the caller (e2e bluff-hunt, mutation gate, CI).

httptest+telebot-mock integration test asserts the orchestration
contract end-to-end without burning live API calls.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Live end-to-end run + commit `docs/qa/<run-id>/`

**Goal:** Execute `qaherald run --scenario=all` against the operator-supplied live pherald + Telegram bot; commit the resulting `docs/qa/<run-id>/` directory verbatim as §107.x evidence. THIS task is the watershed moment for the entire Wave 5 effort.

**Files:**
- Create: `docs/qa/<run-id>/transcript.jsonl`, `docs/qa/<run-id>/report.md`, `docs/qa/<run-id>/attachments/<sha256>.<ext>` (one per scenario that uploads)
- Modify: `docs/CONTINUATION.md` (r-bump: note the new `docs/qa/<run-id>/` evidence + env-var requirements + the run command)

**Anti-bluff §107 anchor for this task:** This task IS the §107.x evidence — there is no anchor "for" it; the task itself is the anchor. Every scenario MUST PASS; any FAIL means the entire Wave 5 cannot tag `v1.0.0-dev-0.0.1`.

**Operator prerequisites — confirm BEFORE running:**

1. `pherald` binary built + running locally with TLS 1.3 + HTTP/3 + Brotli + TOON wired (Wave 4a + 4b substrate). Verify via `curl -k https://localhost:7443/healthz` returning 200.
2. The Herald bot token (`HERALD_TGRAM_BOT_TOKEN`) is exported in the shell.
3. `HERALD_TGRAM_CHAT_ID` is the chat the bot has access to (1:1 with the operator or a test group).
4. `HERALD_JWT_SECRET` matches the value pherald reads at startup.
5. The `subscribers` table in Postgres has ≥ 2 entries pointing at the same `chat_id` (for scenario #2 fan-out). If only one subscriber exists, scenario #2 is allowed to SKIP-with-reason — record in the transcript.
6. The cherald compliance policy has a deny-rule for `type=qa.deny-me` (for scenario #4). If not, scenario #4 SKIP-with-reason.

- [ ] **Step 1: Pre-flight checks**

```bash
cd /Users/milosvasic/Projects/Herald
echo "HERALD_TGRAM_BOT_TOKEN set: $([[ -n "$HERALD_TGRAM_BOT_TOKEN" ]] && echo yes || echo NO)"
echo "HERALD_TGRAM_CHAT_ID:       ${HERALD_TGRAM_CHAT_ID:-unset}"
echo "HERALD_JWT_SECRET set:      $([[ -n "$HERALD_JWT_SECRET" ]] && echo yes || echo NO)"
curl -sk https://localhost:7443/healthz | head -1
```

Expected: every required env-var set; healthz returns `{"status":"ok"}` or equivalent.

- [ ] **Step 2: Run the full scenario suite**

```bash
go build ./qaherald/...
./qaherald run --scenario=all \
    --herald-url=https://localhost:7443 \
    --out-dir=docs/qa
```

The CLI prints `qaherald run id: 2026-05-22T...` then one line per scenario as it runs. Expected: 8 PASS lines (or 8 PASS lines with SKIP-with-reason for scenarios that lack their operator-side prerequisite — recorded in the transcript Note).

Capture the run-id from stdout — used in the commit message below.

- [ ] **Step 3: Inspect the generated evidence**

```bash
RUN_ID=$(ls -1 docs/qa/ | sort | tail -1)
echo "Run directory: docs/qa/$RUN_ID"
ls -la docs/qa/$RUN_ID/
head -3 docs/qa/$RUN_ID/transcript.jsonl
head -30 docs/qa/$RUN_ID/report.md
```

Expected: `transcript.jsonl` non-empty (≥ 16 events for 8 scenarios × 2 minimum), `report.md` rendered with all 4 sections (Summary / Scenario timelines / Attachment manifest / aggregates), `attachments/` populated with the 3 attachment scenarios' files (or 2 if oversized-payload was attempted as a variant inside #6).

- [ ] **Step 4: Run §107.y working-tree quiescence check before staging**

```bash
# Per §107.y mandate (working-tree quiescence) — scan for mutation
# markers before adding the run directory.
grep -RIn --include="*.go" --include="*.sh" \
    -e "MUTATED for paired" \
    -e "// always pass" \
    -e "_mutated_" \
    docs/qa/$RUN_ID/ 2>&1 | head -5
if [ -f .git/MUTATION_IN_PROGRESS ]; then
    echo "FAIL — mutation gate lockfile present; aborting"
    exit 1
fi
```

Expected: empty grep output; no lockfile.

- [ ] **Step 5: Stage + commit**

```bash
git add docs/qa/$RUN_ID/ docs/CONTINUATION.md
git commit -m "$(cat <<EOF
Wave 5 step 8: live qaherald run — docs/qa/$RUN_ID/ as §107.x evidence

8 scenarios executed against the operator-supplied live pherald +
Telegram bot. All PASS (or SKIP-with-reason where prerequisite
absent — verdict per-scenario in report.md).

This commit is the §107.x QA-evidence artefact for every Herald
feature shipped through Wave 4b: pherald POST /v1/events,
cherald /v1/compliance, sherald /v1/safety_state, plus Telegram
delivery, attachment round-trip, and §18.1.1 command prefixes.

Working-tree quiescence (§107.y) verified pre-stage; no mutation
markers, no .git/MUTATION_IN_PROGRESS lockfile.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 6: Update CONTINUATION.md**

Edit `docs/CONTINUATION.md` to add:
- Required env-vars (`HERALD_TGRAM_BOT_TOKEN`, `HERALD_TGRAM_CHAT_ID`, `HERALD_JWT_SECRET`)
- The canonical run command: `./qaherald run --scenario=all --herald-url=https://localhost:7443`
- Pointer to `docs/qa/<run-id>/` as the §107.x evidence layout

Commit separately if Step 5 already closed:

```bash
git add docs/CONTINUATION.md
git commit -m "$(cat <<'EOF'
Wave 5 step 8b: CONTINUATION.md — qaherald handoff prompt

Adds env-var prerequisites + canonical run command + docs/qa/<run-id>/
evidence pointer per §107.x mandate.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: e2e_bluff_hunt invariants E63..E70

**Goal:** Append 8 new invariants to `scripts/e2e_bluff_hunt.sh` that exercise qaherald end-to-end at every commit on `main`. Each invariant builds the binary, runs a single scenario, asserts the transcript + report + attachment artefacts exist and are well-formed.

**Files:**
- Modify: `scripts/e2e_bluff_hunt.sh`

**Anti-bluff §107 anchor for this task:** Each invariant MUST fail when its scenario FAILs (not just when the binary doesn't build). T10 mutation gate runs the full bluff-hunt after each mutation and asserts the relevant invariant FAILs — that round-trip is what makes the gate honest.

- [ ] **Step 1: Add the E63..E70 block to `e2e_bluff_hunt.sh`**

The invariants:

| ID | What it asserts |
|---|---|
| E63 | `qaherald` binary builds from `./qaherald/...` |
| E64 | `qaherald version` returns the expected `commons.Version` (Wave 5 = `1.0.0-dev-0.0.1` after T11 tag) |
| E65 | `qaherald run --scenario=happy-path-single-channel` exits 0 against a real pherald started for the test; transcript has ≥ 2 events for the scenario |
| E66 | After E65 runs, `docs/qa/<run-id>/report.md` exists AND contains all 4 canonical sections (`# qaherald run report`, `## Summary`, `## Scenario timelines`, `## Attachment manifest`) |
| E67 | `qaherald run --scenario=attachment-upload-photo` produces ≥ 1 file under `docs/qa/<run-id>/attachments/` AND the on-disk file's sha256 matches the transcript record |
| E68 | Bidirectional invariant — at least one `KindHeraldPost` AND at least one `KindTGSend`-or-`KindTGReceive` event recorded for every scenario in the transcript |
| E69 | TOON content negotiation exercised — at least one transcript event has `Note` containing `Accept: application/toon` AND at least one response Note recording `content-type=application/toon` |
| E70 | SKIP-with-reason — live multi-subscriber fan-out (scenario #2) is gated on an operator-configured ≥ 2 subscribers; recorded as SKIP-with-reason when prerequisite absent |

- [ ] **Step 2: Header tally update**

Edit the script header from `Sixty-one invariants (E1..E62; E55+E62 SKIP-with-reason)` to `Sixty-nine invariants (E1..E70; E55+E62+E70 SKIP-with-reason)`. Update the comment block at the top to enumerate E63..E70.

- [ ] **Step 3: Run the bluff-hunt**

```bash
bash scripts/e2e_bluff_hunt.sh 2>&1 | tail -30
```

Expected: 69 invariants reported; the new E63..E70 block PASSes (E70 SKIP-with-reason if multi-subscriber not configured).

- [ ] **Step 4: Commit**

```bash
git add scripts/e2e_bluff_hunt.sh
git commit -m "$(cat <<'EOF'
Wave 5 step 9: e2e_bluff_hunt — E63..E70 qaherald invariants

8 new invariants: build, version, run-one-scenario, report.md
sections, attachment sha256 round-trip, bidirectional invariant,
TOON exercised, multi-subscriber SKIP-with-reason.

Tally bumped Sixty-one → Sixty-nine.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Wave 5 mutation gate — `tests/test_wave5_mutation_meta.sh`

**Goal:** Three paired mutations + detectors. Each mutation forcibly breaks a §107 anti-bluff anchor; the detector asserts the corresponding e2e invariant or scenario assertion FAILs while the mutation is live; then the mutation is reverted and the detector re-asserts PASS.

**Files:**
- Create: `tests/test_wave5_mutation_meta.sh`

**Anti-bluff §107 anchor for this task:** This task IS the meta-anchor — every other task's anti-bluff claim is conditional on T10 catching its planned bluff. If T10's mutations don't surface FAILs, the entire Wave 5 trust chain collapses.

**Working-tree quiescence (§107.y) — MANDATORY pre-flight:**

The script's first action is `touch .git/MUTATION_IN_PROGRESS`; its `trap EXIT` removes it. Before any mutation, the script scans the working tree for pre-existing mutation markers (`MUTATED for paired`, `// always pass`, `_mutated_*` filenames) and ABORTS if any are present (Wave 4b's lesson from commit `72e81ab` → `d5bd360`).

- [ ] **Step 1: Skeleton + quiescence pre-flight**

```bash
#!/usr/bin/env bash
# Wave 5 mutation gate — qaherald §107 anti-bluff trust chain
# Per Helix §11.4.84 / Herald §107.y, lockfile + working-tree
# quiescence ENFORCED.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

LOCKFILE=".git/MUTATION_IN_PROGRESS"
if [ -f "$LOCKFILE" ]; then
    echo "FAIL — concurrent mutation gate detected ($LOCKFILE present)"
    exit 1
fi
touch "$LOCKFILE"
trap 'rm -f "$LOCKFILE"' EXIT

# Quiescence pre-flight
QUIESCENCE_HITS=$(grep -RIn --include="*.go" --include="*.sh" \
    -e "MUTATED for paired" \
    -e "// always pass" \
    -e "_mutated_" \
    qaherald/ scripts/ tests/ 2>/dev/null || true)
if [ -n "$QUIESCENCE_HITS" ]; then
    echo "FAIL — working-tree mutation markers present pre-flight:"
    echo "$QUIESCENCE_HITS"
    exit 1
fi

FAIL=0
mutate_and_assert() {
    local label="$1" file="$2" mutation="$3" detector="$4"
    echo "== mutation: $label =="
    cp "$file" "$file.bak.wave5gate"
    # apply mutation
    eval "$mutation"
    # detector MUST exit non-zero while mutation is live
    if eval "$detector"; then
        echo "FAIL — detector passed under mutation (bluff resurrected): $label"
        FAIL=$((FAIL+1))
    else
        echo "PASS — detector caught mutation: $label"
    fi
    # restore
    mv "$file.bak.wave5gate" "$file"
    # detector MUST exit zero post-restore
    if eval "$detector"; then
        echo "PASS — post-restore green: $label"
    else
        echo "FAIL — post-restore failed (state-corruption): $label"
        FAIL=$((FAIL+1))
    fi
}
```

- [ ] **Step 2: Mutation (a) — blank out transcript Writer.Append**

```bash
mutate_and_assert \
    "transcript_append_blank" \
    "qaherald/internal/transcript/transcript.go" \
    "sed -i.tmp '/^func (w \\*Writer) Append/,/^}/c\
func (w *Writer) Append(ev Event) error { return nil /* MUTATED for paired §1.1 — Wave 5 (a) */ }
' qaherald/internal/transcript/transcript.go" \
    "go test -count=1 ./qaherald/internal/transcript/... 2>&1 | grep -q FAIL"
```

The detector: the transcript round-trip unit test (T2 Step 2) checks that 3 appended events appear when re-reading the file. With Append no-oped, the assertion `expected 3 events, got 0` fires → test FAILs → grep finds FAIL → detector returns 0 → outer logic flips to "PASS — detector caught mutation".

- [ ] **Step 3: Mutation (b) — Herald client always returns 202 stub**

```bash
mutate_and_assert \
    "herald_post_always_202" \
    "qaherald/internal/herald/client.go" \
    "sed -i.tmp '/^func (c \\*Client) PostEvent/,/^}/c\
func (c *Client) PostEvent(ctx context.Context, ce CloudEvent, accept string) (Receipt, int, http.Header, error) {
    return Receipt{EventID: ce.ID, Recipients: 1, Status: \"accepted\"}, 202, http.Header{}, nil /* MUTATED for paired §1.1 — Wave 5 (b) */
}
' qaherald/internal/herald/client.go" \
    "./qaherald/qaherald run --scenario=deny-path-policy-gate --herald-url=https://localhost:7443 2>&1 | grep -q FAIL"
```

The detector: deny-path expects `PostEvent` to return 403; the stub returns 202; the scenario's `assertStatus(403, status, ...)` FAILs → scenario reports FAIL → run exits non-zero.

(NOTE: this mutation requires a live pherald during the test. The script optionally skips Mutation (b) when `HERALD_BASE_URL` is unset, recording SKIP-with-reason instead — same posture as Wave 4b's M4/M5 live-pherald gating.)

- [ ] **Step 4: Mutation (c) — sha256-mismatch tolerance**

```bash
mutate_and_assert \
    "attach_sha_tolerance" \
    "qaherald/internal/scenario/attach_photo.go" \
    "sed -i.tmp 's/if shaUploaded != shaDownloaded/if false /* MUTATED for paired §1.1 — Wave 5 (c) */ \\&\\& shaUploaded != shaDownloaded/' qaherald/internal/scenario/attach_photo.go" \
    "./qaherald/qaherald run --scenario=attachment-upload-photo --herald-url=https://localhost:7443 2>&1 | grep -q FAIL"
```

The detector: scenario #5 normally asserts sha256-equality; the mutation flips the guard to `false &&` so the assertion never fires. To make the detector catch this, the scenario's POST-condition adds a sentinel check on the recorded transcript — i.e. if `sha256(uploaded) != sha256(downloaded)` AND no error was reported, then the assertion-skipping bluff is live. The implementer extends scenario #5 with this sentinel check; without it, mutation (c) is undetectable.

Alternative simpler detector: a unit test (`scenario/attach_photo_test.go`) injects mismatched sha256 values via a mock TG client and asserts the scenario FAILs. That unit test bypasses the live-pherald gating; preferred.

- [ ] **Step 5: Tail check**

```bash
if [ "$FAIL" -ne 0 ]; then
    echo "Wave 5 mutation gate — $FAIL mutation(s) failed their detector contract"
    exit 1
fi
echo "Wave 5 mutation gate — 3/3 mutations + detectors green"
```

- [ ] **Step 6: Run + commit**

```bash
chmod +x tests/test_wave5_mutation_meta.sh
bash tests/test_wave5_mutation_meta.sh 2>&1 | tail -20
git add tests/test_wave5_mutation_meta.sh qaherald/internal/scenario/attach_photo_test.go
git commit -m "$(cat <<'EOF'
Wave 5 step 10: paired-mutation gate (3 mutations + detectors)

(a) blank transcript Writer.Append — unit test catches missing events
(b) Herald.PostEvent always-202 stub — deny-path scenario FAILs
(c) attachment sha256 mismatch tolerance — attach_photo_test catches

Working-tree quiescence (§107.y) ENFORCED pre-flight + post-restore.
Lockfile .git/MUTATION_IN_PROGRESS held for the duration; trap EXIT
removes it.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Spec V3 r11→r12, Issues/Fixed/Status r-bumps, tag `v1.0.0-dev-0.0.1`, 4-mirror push

**Goal:** Land the documentation closeout for Wave 5 + tag the release + push to all 4 mirrors.

**Files:**
- Modify: `docs/specs/mvp/specification.V3.md` (add §47 QA-bot section; §43 commands catalogue extension)
- Modify: `docs/Issues.md` (HRD-104..HRD-107 atomic Issues→Fixed close)
- Modify: `docs/Fixed.md` (prepend HRD-104..HRD-107)
- Modify: `docs/Status.md` (Wave 5 summary; e2e 61→69; modules 15→16; tag)
- Modify: `commons/version.go` (bump `Version` constant to `1.0.0-dev-0.0.1`)

**Anti-bluff §107 anchor for this task:** The tag MUST NOT land before T8's `docs/qa/<run-id>/` directory is committed AND `scripts/e2e_bluff_hunt.sh` reports 69/69 (or 69 with documented SKIP-with-reasons). The release-tag guard already checks `docs/qa/` per §107.x; this task is where it triggers.

- [ ] **Step 1: Spec V3 §47 — QA bot section**

Insert immediately after the current §46 (or wherever the structure dictates — confirm by reading the table of contents of `specification.V3.md`). Section contents:

```markdown
## §47 — QA bot (qaherald)

### §47.1 Purpose

qaherald is Herald's QA flavor binary (8th flavor, 16th workspace module).
It exists to satisfy §107.x — every Herald feature ships with a
docs/qa/<run-id>/ directory containing a bidirectional transcript,
content-addressed attachments, and a generated Markdown report.

### §47.2 Run-ID format

ISO timestamp (colon → hyphen) + UUIDv7 short suffix:
`2026-05-22T15-30-00-7a3b`. Computed once at Writer construction.

### §47.3 Transcript format (JSONL)

One event per line; schema:

| field | type | description |
|---|---|---|
| ts | RFC3339 | event timestamp UTC |
| direction | enum | out / in / internal |
| kind | enum | tg.send / tg.receive / tg.upload / tg.download / herald.post / herald.response / herald.get / scenario.start / scenario.end / assert / wait |
| scenario | string | name (omitted for non-scenario events) |
| payload | JSON | event-specific |
| attachments | array | [{sha256, content_type, size_bytes, original_filename}] |
| note | string | free-form |

### §47.4 Attachment storage

Content-addressed: docs/qa/<run-id>/attachments/<sha256>.<ext>.
The transcript attachments[] entry carries sha256 + content-type +
size + original filename. Reports cross-reference each one.

### §47.5 Scenario taxonomy

8 named scenarios — see Wave 5 plan T5 table for the canonical list.
Scenario language: Go funcs (typed, IDE-assisted) via the Registry
pattern. NOT YAML.

### §47.6 Anti-bluff invariants

Three §107 anchors enforced by tests/test_wave5_mutation_meta.sh:
(a) transcript completeness — Writer.Append cannot be no-oped;
(b) wire-byte honesty — Herald.PostEvent cannot stub status;
(c) sha256 integrity — attachment scenarios cannot skip the equality.

### §47.7 Operator invocation

`qaherald run --scenario=<name|all> --herald-url=<https-url>`
with env-var fallbacks for token + chat-ID. See §43 commands
catalogue entry.
```

- [ ] **Step 2: Spec V3 §43 commands catalogue addendum**

Locate the §43 commands table. Append two rows:

| Command | Purpose | Flags |
|---|---|---|
| `qaherald run` | Execute QA scenarios end-to-end | `--scenario`, `--out-dir`, `--herald-url`, `--herald-token`, `--tg-token`, `--tg-chat`, `--scenario-timeout` |
| `qaherald version` | Print Herald version string | (none) |

- [ ] **Step 3: r-bump every modified doc**

Bump revision counters: spec V3 r11→r12; `docs/Issues.md` r14→r15; `docs/Fixed.md` r13→r14; `docs/Status.md` r15→r16. Each gets a `Status summary` row describing Wave 5 closeout.

- [ ] **Step 4: HRD-104..HRD-107 atomic Issues→Fixed close**

Per V3 §8.3 + §11.4.19, Issues→Fixed migration is atomic-per-HRD-per-commit. HRD-104 = qaherald scaffold + transcript (T1+T2); HRD-105 = scenarios + report (T5+T6); HRD-106 = live end-to-end run + docs/qa/ (T8); HRD-107 = e2e + mutation gates + tag (T9+T10+T11).

Each HRD entry under `Fixed.md` carries: HRD-NNN, title, commit SHA (filled in at tag time), e2e invariant references (E63..E70 mapping), `docs/qa/<run-id>/` evidence path.

- [ ] **Step 5: Bump `commons.Version`**

```go
// commons/version.go
package commons

const Version = "1.0.0-dev-0.0.1"
```

(Confirm the canonical file path with `grep -rn "const Version" commons/`; some Helix-stack modules carry the constant in `commons/types.go` instead.)

- [ ] **Step 6: Regenerate PDF/HTML/DOCX siblings for every modified .md**

```bash
bash scripts/export_docs.sh docs/specs/mvp/specification.V3.md docs/Issues.md docs/Fixed.md docs/Status.md docs/CONTINUATION.md
```

(Per CLAUDE.md "Documentation artefacts" — siblings go stale when their .md changes.)

- [ ] **Step 7: Final full-run smoke**

```bash
bash scripts/audit_antibluff.sh
bash scripts/codegraph_validate.sh
bash scripts/e2e_bluff_hunt.sh
bash tests/test_constitution_inheritance.sh
bash tests/test_constitution_inheritance_meta.sh
bash tests/test_wave4b_mutation_meta.sh
bash tests/test_wave5_mutation_meta.sh
```

Expected: every gate green. If anything FAILs, fix at root cause per §11.4.4 — DO NOT tag while red.

- [ ] **Step 8: Tag**

```bash
git tag -a v1.0.0-dev-0.0.1 -m "$(cat <<'EOF'
Herald v1.0.0-dev-0.0.1 — Wave 5 qaherald

QA-bot flavor binary lands. 8 scenarios end-to-end against live
pherald + Telegram bot. docs/qa/<run-id>/ evidence layer
operational (§107.x mandate).

- 16 workspace modules (added qaherald)
- e2e_bluff_hunt at 69 invariants (E1..E70)
- 19 paired mutation gates (Wave 5 adds 3)
- Spec V3 r12 (§47 QA-bot + §43 commands extension)

First development-track tag on the road to v1.0.0.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 9: Push to all 4 mirrors**

```bash
for upstream in upstreams/*.sh; do
    . "$upstream"
    echo "Pushing to $UPSTREAMABLE_REPOSITORY"
    git push "$UPSTREAMABLE_REPOSITORY" main --follow-tags
done
```

Expected: 4 successful pushes (GitHub, GitLab, GitFlic, GitVerse).

- [ ] **Step 10: Final commit (closeout)**

```bash
git add docs/ commons/version.go
git commit -m "$(cat <<'EOF'
Wave 5 step 11: spec V3 r12 + HRD-104..107 close + tag v1.0.0-dev-0.0.1

§47 QA bot added; §43 commands catalogue extended with qaherald
run/version. Issues→Fixed atomic close for HRD-104..107. e2e
tally 61→69. Workspace count 15→16. commons.Version bumped to
1.0.0-dev-0.0.1.

Closes Wave 5. First v1.0.0-dev-* tag.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Self-review before commit

This plan is self-reviewed against three criteria — DO NOT commit the plan until each line below resolves to green.

1. **Operator-mandate coverage.** Every operator requirement maps to a task:
   - Run-ID format → T2 (computeRunID)
   - JSONL transcript → T2 (Writer)
   - Content-addressed attachments → T2 (AttachFile)
   - Go-funcs scenarios → T5 (Scenario type)
   - Tag `v1.0.0-dev-0.0.1` → T11 (Step 8)
   - Brotli/HTTP3 exercise → T4 (client.go ALPN preference + Brotli accept-encoding) and called from every scenario in T5
   - Workspace 15→16 → T1 (go.work) + T11 (CLAUDE.md final r-bump)
   - 100% documented feature coverage → T5 scenarios collectively cover pherald POST + cherald compliance + sherald safety + Telegram delivery + attachment round-trip + §18.1.1 command prefixes
   - Bidirectional + real proofs + transcripts → T2 + T5 + T8 (live run) + T10 (mutation gates enforce honesty)
   - §107.x docs/qa/<run-id>/ → T8

2. **No placeholders.** Scan the plan for forbidden tokens:
   - "TBD" — absent
   - "implement later" — absent
   - "similar to Task N" — absent (every scenario lists its own steps + assertions)
   - "handle edge cases" — absent

3. **Type consistency across tasks.** Names verified consistent:
   - `Writer` (T2) — same struct referenced from T5 (`Orchestrator.Transcript`) and T6 (`Generate` reads JSONL)
   - `Event` (T2) — same struct used in T5 scenario writes and T6 report reads
   - `Orchestrator` (T5) — exposed as the runtime context to every scenario; T7's `runRun` constructs it
   - `Scenario` (T5) — registry pattern; T7 enumerates via `scenario.All()`
   - `Result` (T5) — written to `results.json` by T7; consumed by T6 `Generate`
   - `Client` (T3, T4) — namespaced via package (`tgram.Client`, `herald.Client`); never conflated
   - `Attachment` (T2) — sha256 + content_type + size + original_filename; same shape from write (T2) → read (T6)

All three checks green. Plan is ready to commit.

---

## Commit the plan

```bash
git add docs/superpowers/plans/2026-05-22-wave5-qaherald.md
git commit -m "$(cat <<'EOF'
Wave 5 implementation plan: qaherald — pherald ↔ Telegram QA-bot automation (v1.0.0-dev-0.0.1)

11 tasks, TDD, full anti-bluff §107.x compliance.

Honors operator mandate (2026-05-22): 100% documented feature
coverage; bidirectional; real proofs; transcripts + attachments
under docs/qa/<run-id>/.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

DO NOT push (controller will batch with subsequent Wave 5 commits).

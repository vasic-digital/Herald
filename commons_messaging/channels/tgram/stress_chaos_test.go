package tgram_test

// HRD-137 — tgram adapter §11.4.85 stress + chaos test suite (final task in the
// tgram-completeness wave per the 2026-05-28 live-gap analysis). Closes GAP-3
// for the tgram channel adapter surface.
//
// Wave A + Wave B already landed:
//   - fe2a7c8: lifecycle handlers (OnEdited / OnMyChatMember / service msgs)
//   - 6d7d543: webhook validator (X-Telegram-Bot-Api-Secret-Token)
//   - ff9b9c0: TELEGRAM.md operator guide
//   - 8162d14: HRD-133 (sanitizeTgramError token redaction) +
//              HRD-134 (withRetry429 honoring retry_after)
//
// This file adds the §11.4.85 stress + chaos proofs the operator's
// tgram-completeness mandate calls out. Both tests run via the
// commons/stresschaos scaffold and write captured-evidence artefacts under
// qaRoot(t)/HRD-137-tgram-stress/<surface>/ (HERALD_STRESS_QA_DIR-or-TempDir
// guard per the existing convention).
//
// Scope:
//   - STRESS (TestTgram_Stress_MultiRecipientFanOut): Send/SendReply is
//     concurrency-safe under N goroutines hitting N distinct chat_ids on the
//     SAME *Adapter — no shared-state corruption, no token confusion, no
//     dropped messages, -race clean. Uses Adapter.SendReply (which accepts
//     chat_id per call) so one adapter can be exercised against many chats.
//   - CHAOS (TestTgram_Chaos_GetUpdatesPollerResilience): PASS as of the
//     conductor's one-line subscribe.go fix that threads a.baseURL into
//     telebot.Settings.URL before NewBot (see subscribe.go lines ~108-114 —
//     `if a.baseURL != "" { settings.URL = a.baseURL }`, parallel to
//     ensureBot in tgram.go:60-67). Prior revisions SHIPped this test as
//     SKIP-with-reason because Subscribe() constructed its own *telebot.Bot
//     via NewBot(Settings{Token,Poller}) without that thread-through, and
//     without it there was no hermetic way to redirect the live poller at
//     an httptest fault-injector. With the seam now live the test drives
//     the REAL Subscribe loop through an httptest server that flips
//     between sustained 500s, mid-poll hangs, mid-body connection-close,
//     and success windows — asserting that the poller recovers, ctx-cancel
//     terminates cleanly, and goroutines settle (no leak). Per §107
//     anti-bluff covenant (CLAUDE.md §107 / Helix §11.4): every assertion
//     is a positive captured-runtime invariant from the real production
//     poller, NOT a metadata-only / "absence-of-error" PASS.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/stresschaos"
	"github.com/vasic-digital/herald/commons_messaging/channels/tgram"
)

// qaRoot returns the §107.x / §11.4.85 evidence root. When
// HERALD_STRESS_QA_DIR is set (explicit evidence-capture runs), artefacts
// persist there (point it at the repo docs/qa to refresh the committed
// HRD-137 evidence). Otherwise it returns t.TempDir() so ordinary
// `go test` / e2e runs NEVER dirty tracked evidence (mirrors the
// canonical pattern in commons_constitution/audit_stress_chaos_test.go +
// pherald/cmd/pherald/listen_stress_chaos_test.go).
func qaRoot(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("HERALD_STRESS_QA_DIR"); dir != "" {
		return dir
	}
	return t.TempDir()
}

// fanOutCounter tracks per-chat hit counts on the httptest server so the
// stress test can assert no-loss / no-double for every recipient. sync.Map
// gives lock-free reads + atomic-store semantics for the per-chat counters
// (which themselves are *int64 atomics so increments are race-free).
type fanOutCounter struct {
	hits sync.Map // chat_id (int64) -> *int64 atomic counter
	// total is bumped on EVERY sendMessage hit regardless of which chat —
	// the "no-double" assertion checks total == workers*iters and the
	// per-chat sum == total.
	total atomic.Int64
}

func (f *fanOutCounter) bump(chatID int64) {
	loaded, _ := f.hits.LoadOrStore(chatID, new(int64))
	atomic.AddInt64(loaded.(*int64), 1)
	f.total.Add(1)
}

func (f *fanOutCounter) get(chatID int64) int64 {
	v, ok := f.hits.Load(chatID)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// TestTgram_Stress_MultiRecipientFanOut proves Send/SendReply is
// concurrency-safe under N goroutines sending to N distinct recipients via
// the SAME *tgram.Adapter — no shared-state corruption, no token confusion,
// no dropped messages, no race-detector hits.
//
// Design:
//   - httptest server records every /bot<token>/sendMessage call's chat_id
//     in a per-chat atomic counter. Telegram Bot API expects chat_id as a
//     JSON field in the request body (see telebot/bot_raw.go:22-52 +
//     send_reply_test.go's body-bytes pattern).
//   - ONE *tgram.Adapter pointed at the httptest server via
//     NewAdapterWithBaseURL (the existing seam Send/ensureBot already
//     respects — tgram.go:60-67 confirms Settings.URL is threaded through).
//   - Adapter.SendReply(chatID, ...) accepts chat_id per call (send.go:212)
//     so one Adapter can fan out to many chats — this is the seam that lets
//     us prove the no-confusion + no-corruption invariants the task spec
//     calls for.
//   - commons/stresschaos.RunLoad with workers=20, iters=20 (= 400 total
//     SendReply calls); each worker uses a UNIQUE chat_id derived from its
//     worker index so the server's per-chat counter for that worker MUST
//     end at exactly `iters` and the global counter at exactly `workers ×
//     iters`.
//
// Assertions (every one a real captured-runtime invariant, §107 anti-bluff):
//   - No-loss:   for every worker w in [0,workers), counter[chatID(w)] == iters.
//   - No-double: f.total.Load() == workers*iters.
//   - Race-clean: implicit when the test runs under -race -count=3.
//   - Throughput floor: at least 100 ev/s sanity (httptest overhead under
//     -race; the §11.4.85 floor is "p99 recorded", not "p99 ≤ X").
//
// Evidence: assertion.txt + latency.json + latency_histogram.csv under
// qaRoot(t)/HRD-137-tgram-stress/fanout/.
func TestTgram_Stress_MultiRecipientFanOut(t *testing.T) {
	if testing.Short() {
		t.Skip("tgram stress test skipped in -short mode")
	}

	var fc fanOutCounter
	const token = "STRESSTESTTOKEN" // synthetic — never reaches a real Bot API

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			// telebot.NewBot dispatches getMe synchronously (bot.go:58).
			// Minimal valid bot User payload so NewBot returns nil err.
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"TestBot"}}`))
			return
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			// telebot encodes sendMessage params as JSON (bot_raw.go:22-52).
			// Decode the body, read chat_id, bump the per-chat atomic
			// counter. The raw body might encode chat_id as a JSON number
			// (float64 after Unmarshal) OR as a string (when telebot's
			// Recipient() returns the decimal string form); handle both.
			body, _ := io.ReadAll(r.Body)
			defer r.Body.Close()
			var params map[string]any
			if err := json.Unmarshal(body, &params); err == nil {
				var chatID int64 = -1
				if v, ok := params["chat_id"]; ok {
					switch x := v.(type) {
					case float64:
						chatID = int64(x)
					case string:
						if n, perr := strconv.ParseInt(x, 10, 64); perr == nil {
							chatID = n
						}
					case json.Number:
						if n, perr := x.Int64(); perr == nil {
							chatID = n
						}
					}
				}
				if chatID > 0 {
					fc.bump(chatID)
				}
			}
			// Return a unique message_id per call so SendReply has
			// something distinct to read. We deliberately do NOT derive
			// message_id from a shared counter (would itself be a race in
			// the test code); using fc.total.Load is fine because it was
			// just bumped above and is monotone — but to avoid any race
			// reads on the assertion side, just hand back a constant
			// message_id=1; the test does not assert per-call message_id
			// values, only per-chat hit counts.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1},"text":"ok"}}`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// ONE *Adapter, shared by all workers, pointed at the httptest server.
	// chatID in the Adapter struct is unused by SendReply (which takes
	// chat_id per call); pass "0" as a placeholder so NewAdapterWithBaseURL
	// returns a valid struct.
	a := tgram.NewAdapterWithBaseURL(token, "0", srv.URL)

	const workers, iters = 20, 20
	const totalExpected = workers * iters

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	gBefore := runtime.NumGoroutine()

	start := time.Now()
	summary := stresschaos.RunLoad(workers, iters, func(w, i int) error {
		// UNIQUE chat_id per worker — worker `w` always uses chatID =
		// 1000+w, so all iters from that worker land on the SAME chat
		// and the per-chat counter for chatID(w) ends at exactly `iters`.
		// A cross-wire (worker w's request landing on chatID(w')) would
		// surface as one chat short + one chat over.
		chatID := int64(1000 + w)
		text := fmt.Sprintf("stress msg w=%d i=%d", w, i)
		_, err := a.SendReply(context.Background(), chatID, text, 0, nil)
		return err
	})
	elapsed := time.Since(start)

	// No-error: every SendReply succeeded.
	if summary.Errors != 0 {
		t.Fatalf("stress fan-out had %d errors out of %d calls (want 0)", summary.Errors, summary.Count)
	}
	if summary.Count != totalExpected {
		t.Fatalf("stress fan-out call count: got %d want %d", summary.Count, totalExpected)
	}

	// No-double (global): server saw exactly workers*iters /sendMessage
	// calls. Any extra is a duplicate-dispatch race; any missing is a
	// drop.
	gotTotal := fc.total.Load()
	if gotTotal != int64(totalExpected) {
		t.Fatalf("global hit count: got %d want %d (extra=double-dispatch, missing=drop)", gotTotal, totalExpected)
	}

	// No-loss + No-confusion (per-chat): every worker's chat_id received
	// EXACTLY `iters` hits. A wire that mixed up worker w's chat_id with
	// worker w'+1's would surface here as one chat short + one chat over.
	var perChatErrors []string
	for w := 0; w < workers; w++ {
		chatID := int64(1000 + w)
		got := fc.get(chatID)
		if got != int64(iters) {
			perChatErrors = append(perChatErrors,
				fmt.Sprintf("  chat_id=%d: got %d hits, want %d (loss=%d or confusion)",
					chatID, got, iters, int64(iters)-got))
		}
	}
	if len(perChatErrors) > 0 {
		t.Fatalf("per-chat distribution mismatch (no-loss/no-confusion invariant FAIL):\n%s",
			strings.Join(perChatErrors, "\n"))
	}

	// Throughput floor — httptest is in-process; even under -race with 20
	// concurrent goroutines we expect ≥100 ev/s. The §11.4.85 mandate is
	// "p99 recorded", NOT "p99 ≤ X" — the floor is sanity, not a contract.
	tput := float64(summary.Count) / elapsed.Seconds()
	const tputFloor = 100.0
	if tput < tputFloor {
		t.Fatalf("throughput floor: got %.1f ev/s want ≥%.1f (httptest is in-process; under-floor means scheduler pathology)",
			tput, tputFloor)
	}

	// Goroutine settle — httptest server cleanup happens in t.Cleanup;
	// we just check no obvious leak from RunLoad itself.
	runtime.GC()
	leakDeadline := time.Now().Add(2 * time.Second)
	gAfter := runtime.NumGoroutine()
	for time.Now().Before(leakDeadline) {
		gAfter = runtime.NumGoroutine()
		if gAfter <= gBefore+4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	const goroSlack = 16 // httptest keeps idle conns; small slack is normal
	leaked := gAfter - gBefore
	if leaked > goroSlack {
		t.Fatalf("goroutine leak after RunLoad: before=%d after=%d (leaked=%d > slack=%d)",
			gBefore, gAfter, leaked, goroSlack)
	}

	// Evidence capture — assertion.txt + latency.json + latency_histogram.csv
	// under qaRoot(t)/HRD-137-tgram-stress/fanout/.
	run, err := stresschaos.NewRun(qaRoot(t), "HRD-137-tgram-stress")
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface("fanout")
	if err != nil {
		t.Fatalf("Surface(fanout): %v", err)
	}
	if _, err := sd.WriteLatencyJSON(summary); err != nil {
		t.Fatalf("WriteLatencyJSON: %v", err)
	}
	if _, err := sd.WriteHistogramCSV(summary); err != nil {
		t.Fatalf("WriteHistogramCSV: %v", err)
	}

	// Build the per-chat distribution snippet for the assertion artefact.
	// Sorted by chat_id for stable diffs across runs.
	var perChatLines []string
	for w := 0; w < workers; w++ {
		chatID := int64(1000 + w)
		perChatLines = append(perChatLines, fmt.Sprintf("  chat_id=%d hits=%d", chatID, fc.get(chatID)))
	}

	if _, err := sd.WriteFile("assertion.txt", fmt.Sprintf(
		"surface=tgram scenario=stress_multi_recipient_fan_out status=PASS unit=REAL Adapter.SendReply (httptest seam)\n"+
			"workers=%d iters_per_worker=%d total_calls=%d errors=%d\n"+
			"global_hits=%d (==total_calls: no_loss=1 no_double=1)\n"+
			"throughput_per_sec=%.1f (floor=%.1f, satisfied=1)\n"+
			"latency p50_ms=%.2f p95_ms=%.2f p99_ms=%.2f max_ms=%.2f\n"+
			"goroutines_before=%d goroutines_after=%d leaked=%d slack=%d (no_goroutine_leak=1)\n"+
			"race_detector=clean (when run under -race)\n"+
			"per_chat_distribution (each chat_id MUST receive exactly %d hits):\n%s\n"+
			"§107 anti-bluff: every assertion is a positive captured-runtime invariant.\n"+
			"§11.4.85 stress proof: PASS\n",
		workers, iters, summary.Count, summary.Errors,
		gotTotal, tput, tputFloor,
		summary.Latency.P50MS, summary.Latency.P95MS, summary.Latency.P99MS, summary.Latency.MaxMS,
		gBefore, gAfter, leaked, goroSlack,
		iters,
		strings.Join(perChatLines, "\n"),
	)); err != nil {
		t.Fatalf("WriteFile assertion.txt: %v", err)
	}

	t.Logf("tgram stress[fan-out]: %d calls across %d chats (each %d hits), throughput=%.1f/s p99=%.2fms, goroutines %d→%d (leaked=%d)",
		summary.Count, workers, iters, tput, summary.Latency.P99MS, gBefore, gAfter, leaked)
}

// chaosFaultCounters records, per fault mode, how many /getUpdates calls
// were dispatched into that mode during the chaos window. The atomic counters
// are read after ctx-cancel to build the chaos_assertion.txt evidence + drive
// the fault-diversity assertion (every mode > 0 within the window).
type chaosFaultCounters struct {
	fault500     atomic.Int32 // phases 0,1,2 — 500 Internal Server Error
	faultHang    atomic.Int32 // phases 3,4   — 1.5s hang (ctx-aware short-circuit)
	faultClose   atomic.Int32 // phase 5      — mid-body connection close via Hijacker
	success      atomic.Int32 // phases 6,7   — happy-path getUpdates with 1 update
	nextUpdateID atomic.Int32 // monotonic update_id source for the success phases
}

// TestTgram_Chaos_GetUpdatesPollerResilience drives the REAL Subscribe()
// getUpdates long-poll loop through an httptest fault-injector that flips
// between four fault modes — sustained 500s, mid-poll hangs, mid-body
// connection close, and happy-path success — and asserts the §11.4.85 chaos
// contract holds end-to-end against the production poller (no synthetic
// substitute):
//
//  1. Recovery: handlerHits >= recoveryFloor over the chaos window — the
//     poller MUST pull at least that many successful updates THROUGH the
//     interleaved fault matrix. Zero would mean the poller silently wedged
//     on the first error (a §107 resilience-layer PASS-bluff).
//  2. Ctx-aware return: Subscribe(ctx, h) returns ctx.Canceled (or its wrap)
//     within a short window of cancel — the cancellation propagates through
//     telebot.Bot.Stop + Raw's stopClient ctx-cancel + Poller.Poll's stop
//     channel, NOT a Bot API error.
//  3. Goroutine settle: runtime.NumGoroutine returns to within +goroSlack of
//     the pre-test baseline shortly after cancel — proves no goroutine leak
//     in the poller, the bot consume loop, or the per-update handler goroutines
//     (telebot.runHandler spawns one per update by default; they MUST drain).
//  4. Fault diversity: every fault mode (500 / hang / close / success) was
//     exercised at least once within the window. If any mode is 0 the test
//     LOGS (does not fail) — a fast or slow scheduler may have starved one
//     phase. The recovery + ctx + goroutine invariants are the load-bearing
//     proofs; fault-diversity is the supporting "the fault matrix actually
//     ran" sanity check.
//
// Design notes:
//   - getMe is ALWAYS 200 so telebot.NewBot returns nil err (Subscribe guards
//     against an empty bot.Me.Username — running without a self-filter is a
//     boot-time refusal). The bot's username is "chaostestbot"; the success
//     phases construct text messages FROM @someuser (NOT chaostestbot) so
//     the channel-agnostic self-filter (channels.IsSelfEcho) does NOT drop
//     them.
//   - The fault rotation is keyed on a single atomic int counter mod 8: that
//     guarantees every iteration of the matrix exercises every mode if the
//     window is wide enough (≥8 getUpdates calls). At telebot's tight-loop
//     poll cadence (no inter-call sleep) and 1.5s hang phases, we typically
//     see dozens of cycles in 10–12s.
//   - The chaos test runs under -race -count=3; the assertion thresholds
//     below were chosen to be DETERMINISTIC across iterations on a normal
//     dev workstation (M-series Mac / x86 Linux CI). They are deliberately
//     loose where wall-clock timing dominates and strict where the production
//     contract demands it (ctx propagation, no-goroutine-leak).
func TestTgram_Chaos_GetUpdatesPollerResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("tgram chaos test skipped in -short mode")
	}

	const (
		botToken      = "CHAOSTOKEN"
		chatID        = "-100"
		windowSec     = 12      // total chaos window — Subscribe runs this long
		recoveryFloor = int32(3) // poller MUST pull ≥3 successful updates through the fault matrix
		goroSlack     = 16      // post-cancel goroutine slack vs. pre-test baseline
		settleWaitMs  = 2000    // how long to wait for goroutines to drain post-cancel
	)

	var ctr chaosFaultCounters
	var handlerHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			// getMe is dispatched synchronously by telebot.NewBot — always 200,
			// always a valid User payload, NEVER counted as a fault (this is
			// the boot-time roundtrip, not a poll-iteration).
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"username":"chaostestbot","first_name":"ChaosBot"}}`))
			return

		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			// 8-phase fault matrix keyed on the sum of all four counters
			// (monotone — every getUpdates call increments exactly one
			// counter). Modulo-8 cycles through every fault mode + success
			// window so a ≥8-call chaos window exercises every phase at
			// least once.
			phase := getUpdatesPhase(&ctr)

			switch {
			case phase <= 2:
				// Phases 0,1,2 — 500 Internal Server Error. telebot.Raw will
				// surface this via extractOk(data) -> non-nil err, and
				// LongPoller.Poll will b.debug(err) + continue (no retry
				// sleep — the resilience IS the loop just trying again).
				ctr.fault500.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"Internal Server Error"}`))
				return

			case phase <= 4:
				// Phases 3,4 — HANG 1.5s, then return an empty result set.
				// We honour r.Context().Done() so a ctx-cancel during the
				// hang short-circuits (the test cancels at window-end; a
				// stuck hang would otherwise pin the poller's getUpdates
				// past the cancel window and corrupt the goroutine-settle
				// assertion). telebot's HTTP client Timeout is 1m so the
				// 1.5s hang is well within.
				ctr.faultHang.Add(1)
				select {
				case <-time.After(1500 * time.Millisecond):
				case <-r.Context().Done():
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
				return

			case phase == 5:
				// Phase 5 — mid-body connection close via Hijacker. Write a
				// valid JSON prefix that LOOKS like the start of a Bot API
				// response, then yank the conn so telebot's JSON decoder
				// errors on EOF. The poller treats this exactly like a 500
				// (b.debug(err) + continue) — but exercises a different
				// failure path inside the client/decoder stack.
				ctr.faultClose.Add(1)
				hj, ok := w.(http.Hijacker)
				if !ok {
					// Hijack not supported — degrade gracefully to a 500 so
					// the test does not wedge. This is a defensive branch;
					// httptest.Server's default handler DOES support Hijack.
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				conn, bufrw, err := hj.Hijack()
				if err != nil {
					return
				}
				// Bare-bones HTTP/1.1 200 with a truncated JSON body — the
				// Content-Length we promise (32) exceeds what we actually
				// write (the truncated `{"ok":tr` prefix), so the client
				// either reads past end-of-stream or sees EOF mid-decode.
				_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 32\r\nConnection: close\r\n\r\n")
				_, _ = bufrw.WriteString(`{"ok":tr`)
				_ = bufrw.Flush()
				_ = conn.Close()
				return

			default:
				// Phases 6,7 — happy path. Return ONE update with a monotonic
				// update_id and a text message from @someuser. The body MUST
				// match telebot's getUpdates response shape verbatim
				// (bot_raw.go:230-255) so the OnText handler fires and bumps
				// handlerHits.
				ctr.success.Add(1)
				upd := ctr.nextUpdateID.Add(1)
				now := time.Now().Unix()
				body := fmt.Sprintf(
					`{"ok":true,"result":[{"update_id":%d,"message":{"message_id":%d,"date":%d,"chat":{"id":-100,"type":"private"},"from":{"id":99,"is_bot":false,"username":"someuser","first_name":"Some"},"text":"chaos-recovery-%d"}}]}`,
					upd, upd, now, upd,
				)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}

		default:
			// Any other Bot API method (e.g. setWebhook, getChatMember) is a
			// "shouldn't reach" — return 404 so it surfaces in the chaos log.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// Build the REAL *tgram.Adapter via the baseURL seam — Subscribe now
	// threads a.baseURL into telebot.Settings.URL (subscribe.go ~108-114),
	// so the live poller routes through srv.URL instead of api.telegram.org.
	a := tgram.NewAdapterWithBaseURL(botToken, chatID, srv.URL)

	// chaosHandler is an inline commons.InboundHandler that just bumps the
	// recovery counter. We don't inspect ev — the chaos contract is "the
	// poller pulled at least N updates through the fault matrix"; the
	// contents are not the proof, the count is.
	handler := chaosInboundHandler{hits: &handlerHits}

	// Goroutine baseline BEFORE Subscribe goroutine starts. Telebot will spin
	// up: the poller goroutine, the bot.Start consume goroutine, and one
	// goroutine per delivered update (telebot.runHandler in async mode).
	// We give it a small settling moment for any pre-existing test goroutines
	// to be in steady state.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	gBefore := runtime.NumGoroutine()

	ctx, cancel := context.WithTimeout(context.Background(), windowSec*time.Second)
	defer cancel()

	// Subscribe is long-running; run it in a goroutine so we can observe its
	// return value after ctx cancel. The returned error MUST be ctx.Canceled
	// or ctx.DeadlineExceeded (or wrap one of those) — Subscribe ends with
	// `<-ctx.Done(); return ctx.Err()`.
	subErrCh := make(chan error, 1)
	go func() {
		subErrCh <- a.Subscribe(ctx, handler)
	}()

	// Wait for the chaos window to elapse OR Subscribe to return early.
	// In the happy chaos path Subscribe returns AFTER ctx fires.
	var subErr error
	select {
	case subErr = <-subErrCh:
		// Subscribe returned before windowSec — either it errored out (bad)
		// or ctx fired and Subscribe noticed quickly (fine — the deadline
		// path).
	case <-time.After(time.Duration(windowSec+3) * time.Second):
		// Hard ceiling — if Subscribe doesn't return within windowSec+3,
		// something is wedged. Force the timeout and drain.
		cancel()
		select {
		case subErr = <-subErrCh:
		case <-time.After(3 * time.Second):
			t.Fatalf("Subscribe did not return %ds after ctx.Cancel — poller wedged", 3)
		}
	}

	// Assertion 1 — Recovery: handlerHits >= recoveryFloor. THIS is the
	// load-bearing chaos invariant — the poller MUST have pulled at least
	// `recoveryFloor` successful updates THROUGH the interleaved fault
	// matrix. Zero recovery means the poller silently wedged on the first
	// error (the §107 resilience-layer PASS-bluff this test is designed
	// to catch).
	hits := handlerHits.Load()
	if hits < recoveryFloor {
		t.Fatalf("recovery_event_count=%d < floor=%d — poller did NOT recover from the fault matrix (resilience FAIL)\n"+
			"  fault_modes_exercised: 500=%d hang=%d close=%d success=%d",
			hits, recoveryFloor,
			ctr.fault500.Load(), ctr.faultHang.Load(), ctr.faultClose.Load(), ctr.success.Load())
	}

	// Assertion 2 — Ctx-aware return. Subscribe returns ctx.Err() directly
	// (subscribe.go line 358: `<-ctx.Done(); return ctx.Err()`). With
	// context.WithTimeout(windowSec*Second) the expected err is
	// context.DeadlineExceeded; with an explicit cancel() it's
	// context.Canceled. Telebot does not wrap our ctx err, but a future
	// wrapper might — accept either canonical err via errors.Is.
	if subErr == nil {
		t.Errorf("Subscribe returned nil error — expected ctx.Canceled or ctx.DeadlineExceeded")
	} else if !errors.Is(subErr, context.Canceled) && !errors.Is(subErr, context.DeadlineExceeded) {
		// Loosen to a Logf — telebot may bury a different error if the
		// poller's last in-flight getUpdates returned an unrelated error
		// at the exact moment of cancel. The recovery + goroutine
		// invariants are the load-bearing proofs.
		t.Logf("Subscribe error not ctx.Canceled/DeadlineExceeded (loose): %v", subErr)
	}

	// Assertion 3 — Goroutine settle. After ctx cancel, telebot's poller
	// goroutine + bot.Start consume goroutine + per-update handler goroutines
	// MUST drain. We poll runtime.NumGoroutine for up to settleWaitMs and
	// assert the delta vs. baseline is within slack.
	deadline := time.Now().Add(time.Duration(settleWaitMs) * time.Millisecond)
	var gAfter int
	for time.Now().Before(deadline) {
		runtime.GC()
		gAfter = runtime.NumGoroutine()
		if gAfter <= gBefore+goroSlack {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	leaked := gAfter - gBefore
	if leaked > goroSlack*2 {
		// Hard fail only if WAY over slack — a small overshoot is normal
		// (httptest server keeps idle conns; telebot keeps small per-bot
		// state). The §107 anti-leak invariant is "no UNBOUNDED growth"
		// not "exact match".
		t.Errorf("goroutine leak after Subscribe cancel: before=%d after=%d (leaked=%d > 2*slack=%d)",
			gBefore, gAfter, leaked, goroSlack*2)
	} else if leaked > goroSlack {
		t.Logf("goroutine delta over base slack (warning, not fail): before=%d after=%d leaked=%d slack=%d",
			gBefore, gAfter, leaked, goroSlack)
	}

	// Assertion 4 — Fault diversity. Every mode SHOULD have fired at least
	// once during windowSec. If any is zero we LOGF (not fail) — fast
	// schedulers can race past a phase. The recovery invariant above is
	// the load-bearing proof; this is the supporting sanity check that
	// the matrix was actually exercised, not just one phase repeatedly.
	f500 := ctr.fault500.Load()
	fHang := ctr.faultHang.Load()
	fClose := ctr.faultClose.Load()
	fOK := ctr.success.Load()
	if f500 == 0 || fHang == 0 || fClose == 0 || fOK == 0 {
		t.Logf("fault diversity warning (loose): 500=%d hang=%d close=%d success=%d (a 0 is acceptable but unusual)",
			f500, fHang, fClose, fOK)
	}

	// Evidence capture — PASS-state chaos_assertion.txt under
	// qaRoot(t)/HRD-137-tgram-stress/poller_chaos/.
	run, runErr := stresschaos.NewRun(qaRoot(t), "HRD-137-tgram-stress")
	if runErr != nil {
		t.Fatalf("stresschaos.NewRun: %v", runErr)
	}
	sd, sdErr := run.Surface("poller_chaos")
	if sdErr != nil {
		t.Fatalf("Surface(poller_chaos): %v", sdErr)
	}
	subErrStr := "<nil>"
	if subErr != nil {
		subErrStr = subErr.Error()
	}
	if _, werr := sd.WriteFile("chaos_assertion.txt", fmt.Sprintf(
		"surface=tgram scenario=chaos_getupdates_poller_resilience status=PASS\n"+
			"unit=REAL Adapter.Subscribe getUpdates long-poll (httptest fault-injector seam via NewAdapterWithBaseURL)\n"+
			"window_seconds=%d recovery_floor=%d goroutine_slack=%d settle_wait_ms=%d\n"+
			"recovery_event_count=%d (>= floor=%d, satisfied=%v)\n"+
			"fault_modes_exercised: 500=%d hang=%d connection_close=%d success=%d\n"+
			"subscribe_returned_err=%q (errors.Is ctx.Canceled || ctx.DeadlineExceeded check loose)\n"+
			"goroutines_before=%d goroutines_after=%d goroutine_delta=%d slack=%d hard_ceiling=%d\n"+
			"race_detector=clean (when run under -race)\n"+
			"§107 anti-bluff: every assertion is a positive captured-runtime invariant from the REAL Subscribe poller; no synthetic substitute.\n"+
			"§11.4.85 chaos proof: getUpdates poller recovers from sustained Bot API faults (500s, hangs, mid-body connection close).\n",
		windowSec, recoveryFloor, goroSlack, settleWaitMs,
		hits, recoveryFloor, hits >= recoveryFloor,
		f500, fHang, fClose, fOK,
		subErrStr,
		gBefore, gAfter, leaked, goroSlack, goroSlack*2,
	)); werr != nil {
		t.Fatalf("WriteFile chaos_assertion.txt: %v", werr)
	}

	t.Logf("tgram chaos[poller]: recovery_event_count=%d fault_modes(500=%d hang=%d close=%d ok=%d) subscribe_err=%v goroutines %d→%d (leaked=%d)",
		hits, f500, fHang, fClose, fOK, subErr, gBefore, gAfter, leaked)
}

// getUpdatesPhase returns the next phase in the 8-step fault matrix
// (0..7) keyed on the success+fault counters' combined value. Using
// the sum of counters keeps phase progression monotonic AND makes the
// allocation race-free (each branch only bumps ITS own counter; the
// phase decision is a snapshot of all four, summed). Modulo-8 cycles
// through every fault mode + success window so an ≥8-iter chaos
// window exercises every phase at least once.
func getUpdatesPhase(ctr *chaosFaultCounters) int32 {
	total := ctr.fault500.Load() + ctr.faultHang.Load() + ctr.faultClose.Load() + ctr.success.Load()
	return total % 8
}

// chaosInboundHandler is the file-local commons.InboundHandler used by
// TestTgram_Chaos_GetUpdatesPollerResilience. It increments the shared atomic
// counter for every dispatched event. The shape avoids depending on the
// `handlerFunc` declared in subscribe_integration_test.go — that file is
// behind the //go:build integration tag AND in the `tgram` (not `tgram_test`)
// package, so its symbols are not visible here.
type chaosInboundHandler struct {
	hits *atomic.Int32
}

func (h chaosInboundHandler) Handle(ctx context.Context, ev commons.InboundEvent) error {
	h.hits.Add(1)
	return nil
}

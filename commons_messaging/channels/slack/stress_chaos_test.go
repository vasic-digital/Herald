package slack_test

// GAP-G5c — Slack adapter §11.4.85 stress + chaos test suite, mirroring the
// tgram stress_chaos_test.go structure for the SECOND concrete channel adapter.
//
// Both tests are fully hermetic: they drive the REAL *slack.Adapter through an
// httptest server impersonating the Slack Web API — no live tokens, no network.
// Under `go test -race` the concurrent fan-out turns a data race in Send into a
// hard FAIL, so a clean run is positive §11.4.85 concurrency evidence rather
// than a metadata-only PASS.
//
// Scope:
//
//   - TestSlack_Stress_ConcurrentSendFanOut: ONE *slack.Adapter shared across
//     20×20 = 400 concurrent Send calls to distinct channel ids via an
//     httptest server that counts per-channel hits. Asserts every call
//     succeeded, the per-channel hit tallies are exact (no lost/duplicated
//     wire calls under concurrency), a throughput floor, and no goroutine leak.
//
//   - TestSlack_Chaos_PostMessageFaultInjection: the same REAL Send loop driven
//     against an httptest fault-injector that flips between 200-ok,
//     500-server-error, and slack-ok=false:ratelimited responses. Asserts Send
//     surfaces the faults as errors (never a silent bluff PASS), recovers on
//     the next good response, and leaks no goroutines.
//
// Evidence lands under qaRoot(t)/HRD-116-slack-stress/<surface>/ (driven by
// HERALD_STRESS_QA_DIR, else t.TempDir() — same convention as tgram).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons/stresschaos"
	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// qaRoot returns the §107.x / §11.4.85 evidence root. When HERALD_STRESS_QA_DIR
// is set it is used verbatim (operator-driven persistent evidence); otherwise a
// per-test TempDir keeps the hermetic run self-cleaning.
func qaRoot(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("HERALD_STRESS_QA_DIR"); d != "" {
		return d
	}
	return t.TempDir()
}

// TestSlack_Stress_ConcurrentSendFanOut proves Adapter.Send is concurrency-safe
// under N goroutines sending to N distinct channels via ONE shared Adapter.
func TestSlack_Stress_ConcurrentSendFanOut(t *testing.T) {
	const (
		workers   = 20
		iters     = 20
		goroSlack = 16 // httptest keeps idle conns; small slack is normal
	)

	// httptest server records every /chat.postMessage call's channel id.
	var mu sync.Mutex
	perChannel := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.Error(w, "bad path "+r.URL.Path, http.StatusNotFound)
			return
		}
		_ = r.ParseForm()
		ch := r.FormValue("channel")
		mu.Lock()
		perChannel[ch]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": ch, "ts": "1654.0001"})
	}))
	defer srv.Close()

	// ONE *Adapter, shared by all workers, pointed at the httptest server.
	a := slack.NewWithBaseURL("xoxb-test", "", "Cdefault", srv.URL)

	gBefore := runtime.NumGoroutine()

	summary := stresschaos.RunLoad(workers, iters, func(wkr, i int) error {
		channelID := fmt.Sprintf("C%02d", wkr) // distinct channel per worker
		_, err := a.Send(context.Background(), commons.OutboundMessage{
			To:   []commons.Recipient{{Channel: "slack", ChannelUserID: channelID}},
			Body: commons.Body{Plain: fmt.Sprintf("stress w%d i%d", wkr, i)},
		})
		return err
	})

	if summary.Errors != 0 {
		t.Fatalf("RunLoad errors=%d want 0 (§107: every Send must cross the wire and succeed)", summary.Errors)
	}
	if summary.Count != workers*iters {
		t.Fatalf("RunLoad count=%d want %d", summary.Count, workers*iters)
	}

	// Per-channel hit tallies must be EXACT — each worker hit its own channel
	// exactly `iters` times. A lost or duplicated wire call under concurrency
	// would skew these.
	mu.Lock()
	for w := 0; w < workers; w++ {
		ch := fmt.Sprintf("C%02d", w)
		if perChannel[ch] != iters {
			mu.Unlock()
			t.Fatalf("channel %s hits=%d want %d (concurrency lost/duplicated a Send)", ch, perChannel[ch], iters)
		}
	}
	totalChannels := len(perChannel)
	mu.Unlock()
	if totalChannels != workers {
		t.Fatalf("distinct channels=%d want %d", totalChannels, workers)
	}

	// Throughput floor — httptest is in-process; under -race with 20 concurrent
	// goroutines we still expect a healthy floor. Under-floor signals scheduler
	// pathology rather than a correctness bug, but it is worth flagging.
	const floor = 100.0
	if summary.ThroughputPS < floor {
		t.Fatalf("throughput floor: got %.1f ev/s want ≥%.1f (httptest in-process; under-floor => scheduler pathology)",
			summary.ThroughputPS, floor)
	}

	// Goroutine settle — no leak from the concurrent Send fan-out.
	gAfter := runtime.NumGoroutine()
	for try := 0; try < 20 && gAfter > gBefore+goroSlack; try++ {
		time.Sleep(50 * time.Millisecond)
		gAfter = runtime.NumGoroutine()
	}
	if leaked := gAfter - gBefore; leaked > goroSlack {
		t.Fatalf("goroutine leak after RunLoad: before=%d after=%d (leaked=%d > slack=%d)",
			gBefore, gAfter, leaked, goroSlack)
	}

	// Captured-evidence artefact under qaRoot(t)/HRD-116-slack-stress/fanout/.
	writeEvidence(t, "fanout",
		fmt.Sprintf("surface=slack scenario=stress_concurrent_send_fan_out status=PASS unit=REAL Adapter.Send (httptest seam)\n"+
			"calls=%d channels=%d hits_each=%d errors=%d throughput=%.1f/s p50=%.2fms p99=%.2fms\n"+
			"goroutines_before=%d goroutines_after=%d leaked=%d slack=%d (no_goroutine_leak=1)\n",
			summary.Count, totalChannels, iters, summary.Errors, summary.ThroughputPS,
			summary.Latency.P50MS, summary.Latency.P99MS,
			gBefore, gAfter, gAfter-gBefore, goroSlack), summary)

	t.Logf("slack stress[fan-out]: %d calls across %d channels (each %d hits), throughput=%.1f/s p99=%.2fms, goroutines %d→%d",
		summary.Count, totalChannels, iters, summary.ThroughputPS, summary.Latency.P99MS, gBefore, gAfter)
}

// TestSlack_Chaos_PostMessageFaultInjection proves Send surfaces transport +
// platform faults as errors (never a silent bluff PASS) and recovers on the
// next good response, with no goroutine leak across the fault window.
func TestSlack_Chaos_PostMessageFaultInjection(t *testing.T) {
	const goroSlack = 16

	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			http.Error(w, "bad path "+r.URL.Path, http.StatusNotFound)
			return
		}
		n := atomic.AddInt64(&calls, 1)
		switch n % 3 {
		case 1:
			// Healthy 200-ok.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1654.0001"})
		case 2:
			// Transport fault — 500 with no JSON body.
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			// Platform fault — Slack ok=false (rate limited).
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "ratelimited"})
		}
	}))
	defer srv.Close()

	a := slack.NewWithBaseURL("xoxb-test", "", "C123", srv.URL)

	gBefore := runtime.NumGoroutine()

	var okCount, errCount int
	for i := 0; i < 12; i++ {
		_, err := a.Send(context.Background(), commons.OutboundMessage{
			To:   []commons.Recipient{{Channel: "slack", ChannelUserID: "C123"}},
			Body: commons.Body{Plain: fmt.Sprintf("chaos %d", i)},
		})
		if err != nil {
			errCount++
		} else {
			okCount++
		}
	}

	// The fault-injector returns a fault on 2 of every 3 calls. Send MUST
	// surface those as errors — a Send that swallowed the 500 or the ok=false
	// and returned a synthetic success would be a §107 bluff and would inflate
	// okCount here.
	if errCount == 0 {
		t.Fatal("chaos: Send never reported an error despite injected 500 + ok=false faults (§107 bluff — faults silently swallowed)")
	}
	// And it MUST recover — at least one healthy response yields a success.
	if okCount == 0 {
		t.Fatal("chaos: Send never succeeded despite healthy 200-ok responses interleaved (no recovery)")
	}

	// Goroutine settle across the fault window.
	gAfter := runtime.NumGoroutine()
	for try := 0; try < 20 && gAfter > gBefore+goroSlack; try++ {
		time.Sleep(50 * time.Millisecond)
		gAfter = runtime.NumGoroutine()
	}
	if leaked := gAfter - gBefore; leaked > goroSlack {
		t.Fatalf("goroutine leak after chaos: before=%d after=%d (leaked=%d > slack=%d)",
			gBefore, gAfter, leaked, goroSlack)
	}

	writeEvidence(t, "chaos",
		fmt.Sprintf("surface=slack scenario=chaos_postmessage_fault_injection status=PASS unit=REAL Adapter.Send (httptest fault-injector)\n"+
			"sends=12 ok=%d errors=%d (faults_surfaced=1 recovery_observed=1)\n"+
			"goroutines_before=%d goroutines_after=%d leaked=%d slack=%d (no_goroutine_leak=1)\n",
			okCount, errCount, gBefore, gAfter, gAfter-gBefore, goroSlack), stresschaos.LoadSummary{})

	t.Logf("slack chaos[fault-injection]: 12 sends → ok=%d err=%d, goroutines %d→%d", okCount, errCount, gBefore, gAfter)
}

// writeEvidence drops a §11.4.85 captured-evidence artefact (summary.txt +
// latency.json) under qaRoot(t)/HRD-116-slack-stress/<surface>/.
func writeEvidence(t *testing.T, surface, summaryText string, sum stresschaos.LoadSummary) {
	t.Helper()
	run, err := stresschaos.NewRun(qaRoot(t), "HRD-116-slack-stress")
	if err != nil {
		t.Fatalf("stresschaos.NewRun: %v", err)
	}
	sd, err := run.Surface(surface)
	if err != nil {
		t.Fatalf("stresschaos.Surface(%s): %v", surface, err)
	}
	if _, err := sd.WriteFile("summary.txt", summaryText); err != nil {
		t.Fatalf("WriteFile summary.txt: %v", err)
	}
	if sum.Count > 0 {
		if _, err := sd.WriteLatencyJSON(sum); err != nil {
			t.Fatalf("WriteLatencyJSON: %v", err)
		}
	}
}

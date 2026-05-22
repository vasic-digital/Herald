// Wave 5 Task 5 — hermetic unit test for the scenario engine.
//
// Goal: prove the Orchestrator's contract (transcript bidirectionality
// + scenario.start/end emission + PASS Result on a successful body)
// WITHOUT burning live Telegram API quota or hitting a real pherald.
//
// Strategy:
//   - httptest.NewTLSServer stands in for pherald. Returns a canned
//     TOON-encoded Receipt with Recipients=1.
//   - fakeTGSession satisfies the TGSession interface using a buffered
//     channel + a hand-fed canned tele.Message. The test pre-loads the
//     channel with a message whose Text contains the soon-to-be-built
//     CloudEvent ID prefix; WaitForMessage's predicate matches it.
//   - The transcript writer is a real one rooted at t.TempDir() — we
//     assert against the JSONL on disk, not against in-memory state.
//
// §107 anti-bluff anchor: the test's post-condition explicitly
// invokes ValidateScenarioBidirectional on the transcript path AND
// asserts the JSONL contains at least one KindHeraldPost event AND at
// least one KindTGReceive event. This is the same check the T10
// mutation gate uses — a regression that nilled Append would fail
// this test before it could escape to the mutation harness.
package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	toon "digital.vasic.toon/pkg/toon"
	tele "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/qaherald/internal/herald"
	"github.com/vasic-digital/herald/qaherald/internal/transcript"
)

// fakeTGSession is a minimal in-memory TGSession that the orchestrator
// can drive without crossing a network. Send / Upload record their
// inputs into the sent slice (for inspection); WaitForMessage drains
// the pre-loaded inbox channel.
type fakeTGSession struct {
	mu        sync.Mutex
	sent      []string
	uploads   []fakeUpload
	downloads map[string][]byte
	inbox     chan tele.Message
	nextMsgID int
}

type fakeUpload struct {
	contentType string
	filename    string
	body        []byte
}

func newFakeTGSession() *fakeTGSession {
	return &fakeTGSession{
		inbox:     make(chan tele.Message, 16),
		downloads: map[string][]byte{},
		nextMsgID: 1000,
	}
}

func (f *fakeTGSession) Send(text string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	id := f.nextMsgID
	f.nextMsgID++
	return id, nil
}

func (f *fakeTGSession) Upload(r io.Reader, contentType, filename string) (int, string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return 0, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, fakeUpload{contentType: contentType, filename: filename, body: body})
	id := f.nextMsgID
	f.nextMsgID++
	fileID := "fake-file-" + filename
	f.downloads[fileID] = body
	return id, fileID, nil
}

func (f *fakeTGSession) Download(fileID string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.downloads[fileID]
	if !ok {
		return nil, errors.New("fakeTGSession: unknown fileID " + fileID)
	}
	return io.NopCloser(strings.NewReader(string(body))), nil
}

func (f *fakeTGSession) WaitForMessage(timeout time.Duration, predicate func(tele.Message) bool) (tele.Message, error) {
	deadline := time.After(timeout)
	for {
		select {
		case m := <-f.inbox:
			if predicate(m) {
				return m, nil
			}
		case <-deadline:
			return tele.Message{}, context.DeadlineExceeded
		}
	}
}

func (f *fakeTGSession) WaitForReply(timeout time.Duration, toMsgID int, innerPredicate func(tele.Message) bool) (tele.Message, error) {
	return f.WaitForMessage(timeout, func(m tele.Message) bool {
		if m.ReplyTo == nil || m.ReplyTo.ID != toMsgID {
			return false
		}
		if innerPredicate == nil {
			return true
		}
		return innerPredicate(m)
	})
}

// pherald stands in: a TLS test server that decodes the inbound
// CloudEvent (TOON or JSON), pushes the ce.ID onto idCh, and returns
// a canned Receipt that echoes ce.ID back. The test orchestrates
// everything else through ce.ID.
//
// idCh is buffered so the server handler can write without blocking
// on a slow consumer; the test reader uses a select with timeout.
func newPheraldStub(t *testing.T, idCh chan<- string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("server: expected POST, got %s", r.Method)
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("server: read body: %v", err)
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		// Decode into a codec-neutral map to extract the CloudEvent
		// ID — the round-trip through time.Time is brittle in TOON's
		// default formatter (RFC-3339 in, no symmetric out), so we
		// avoid full-struct unmarshal here. The scenario engine
		// itself does not unmarshal CloudEvents — only Receipts —
		// so the production wire path is unaffected.
		var generic map[string]any
		ct := r.Header.Get("Content-Type")
		if toon.IsTOONContentType(ct) {
			if err := toon.Unmarshal(bodyBytes, &generic); err != nil {
				t.Errorf("server: toon.Unmarshal: %v", err)
				http.Error(w, "toon", http.StatusBadRequest)
				return
			}
		} else {
			if err := json.Unmarshal(bodyBytes, &generic); err != nil {
				t.Errorf("server: json.Unmarshal: %v", err)
				http.Error(w, "json", http.StatusBadRequest)
				return
			}
		}
		// TOON encodes Go struct fields by their Go names (not json
		// tag names). Look up both forms so the same handler works
		// for JSON-encoded ("id") and TOON-encoded ("ID") wire
		// payloads.
		idAny := generic["id"]
		if idAny == nil {
			idAny = generic["ID"]
		}
		idStr, _ := idAny.(string)
		select {
		case idCh <- idStr:
		default:
			// Channel full — non-fatal; tests use a buffered chan
			// sized for the expected number of POSTs.
		}
		// Echo back via TOON to match the wire-format the client requested.
		respBody, err := toon.Marshal(herald.Receipt{
			EventID:    idStr,
			Recipients: 1,
			Status:     "accepted",
		})
		if err != nil {
			t.Errorf("server: toon.Marshal: %v", err)
			http.Error(w, "marshal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", herald.AcceptTOON)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(respBody)
	}))
}

// TestRunScenario_HappyPath_Hermetic exercises the full Orchestrator
// path against the fakes + httptest TLS server. Asserts:
//   - Result.PASS == true
//   - Transcript file exists + contains ≥1 herald.* event + ≥1 tg.*
//     event for the scenario name
//   - ValidateScenarioBidirectional reports nil
func TestRunScenario_HappyPath_Hermetic(t *testing.T) {
	t.Parallel()

	tempRoot := t.TempDir()
	w, err := transcript.NewWriter(tempRoot)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	tg := newFakeTGSession()

	// Use a buffered channel so the server handler can hand the
	// CE.ID off to the feeder goroutine without racing on a shared
	// string. Buffer=4 in case future scenarios POST multiple events.
	idCh := make(chan string, 4)
	srv := newPheraldStub(t, idCh)
	defer srv.Close()

	hc := herald.NewWithClient(srv.URL, []byte("test-secret"), srv.Client())

	// Feeder goroutine: drains idCh, pushes one matching Telegram
	// message into tg.inbox per CE seen. The goroutine returns when
	// stop is closed; we drain via select to avoid a leak.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case id, ok := <-idCh:
				if !ok {
					return
				}
				tg.inbox <- tele.Message{
					ID:   42,
					Chat: &tele.Chat{ID: 12345},
					Text: "qaherald delivery for " + id,
				}
			}
		}
	}()

	o := &Orchestrator{
		TG:         tg,
		Herald:     hc,
		Transcript: w,
		ChatID:     12345,
		Now:        func() time.Time { return time.Now().UTC() },
	}

	s, ok := Get("happy-path-single-channel")
	if !ok {
		t.Fatal("happy-path-single-channel scenario not registered")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res := o.RunScenario(ctx, s)

	if !res.PASS {
		t.Fatalf("scenario FAILed: %s", res.ErrorText)
	}
	if res.Scenario != "happy-path-single-channel" {
		t.Fatalf("Result.Scenario: want %q got %q", "happy-path-single-channel", res.Scenario)
	}

	// Re-open the transcript JSONL and count events by Kind for the
	// happy-path scenario. This is the on-disk anti-bluff check —
	// exactly the post-mutation assertion T10 invokes.
	transcriptPath := filepath.Join(w.OutDir(), "transcript.jsonl")
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("transcript file missing: %v", err)
	}

	if err := ValidateScenarioBidirectional(transcriptPath, "happy-path-single-channel"); err != nil {
		t.Fatalf("ValidateScenarioBidirectional: %v", err)
	}

	// Belt-and-braces: re-parse the JSONL and count post + receive
	// events explicitly so a regression that broke the validator
	// itself does not slip past.
	heraldPosts, tgReceives := countEventsByKind(t, transcriptPath, "happy-path-single-channel")
	if heraldPosts < 1 {
		t.Fatalf("expected ≥1 KindHeraldPost, got %d (transcript at %s)", heraldPosts, transcriptPath)
	}
	if tgReceives < 1 {
		t.Fatalf("expected ≥1 KindTGReceive, got %d (transcript at %s)", tgReceives, transcriptPath)
	}
}

// TestValidateScenarioBidirectional_DetectsBluff feeds a synthetic
// blank-Append transcript (zero scenario events) and asserts the
// validator returns the expected §107 violation error. This is the
// inverse check — the T10 mutation gate (a) plants the blank-Append
// mutation; running the validator against the resulting transcript
// MUST surface the bluff.
func TestValidateScenarioBidirectional_DetectsBluff(t *testing.T) {
	t.Parallel()

	tempRoot := t.TempDir()
	w, err := transcript.NewWriter(tempRoot)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	// Append NOTHING for "happy-path-single-channel" — emulating the
	// post-mutation state where Writer.Append is a no-op.
	_ = w.Append(transcript.Event{
		Direction: transcript.DirectionInternal,
		Kind:      transcript.KindWait,
		Scenario:  "different-scenario",
		Note:      "noise to keep the file non-empty",
	})
	_ = w.Close()

	transcriptPath := filepath.Join(w.OutDir(), "transcript.jsonl")
	err = ValidateScenarioBidirectional(transcriptPath, "happy-path-single-channel")
	if err == nil {
		t.Fatal("expected ValidateScenarioBidirectional to fail on bluff-transcript, got nil")
	}
	if !strings.Contains(err.Error(), "bidirectional invariant violated") {
		t.Fatalf("expected 'bidirectional invariant violated' in error, got: %v", err)
	}
}

// countEventsByKind re-reads the transcript JSONL line-by-line and
// returns (heraldPostCount, tgReceiveCount) for the named scenario.
// Used by the happy-path test as a belt-and-braces check independent
// of ValidateScenarioBidirectional.
func countEventsByKind(t *testing.T, transcriptPath, scenarioName string) (int, int) {
	t.Helper()
	body, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var heraldPosts, tgReceives int
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var ev transcript.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		if ev.Scenario != scenarioName {
			continue
		}
		switch ev.Kind {
		case transcript.KindHeraldPost:
			heraldPosts++
		case transcript.KindTGReceive:
			tgReceives++
		}
	}
	return heraldPosts, tgReceives
}

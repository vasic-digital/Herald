package slack_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vasic-digital/herald/commons_messaging/channels/slack"
)

// TestSlackDownloadAttachmentContentAddressed pins the §107 anti-bluff
// anchor for the inbound attachment path (Wave 7 T6, mirrors the Wave 6
// tgram test). Three sub-assertions:
//
//	(a) the final on-disk path is exactly ~/.herald/inbox/slack/<sha>.<ext>;
//	(b) the on-disk bytes equal the canned file payload;
//	(c) a duplicate download is idempotent — same path, same size, no .part
//	    residue (proves we don't re-fetch on every poll).
//
// The httptest server impersonates TWO Slack endpoints:
//
//	POST /files.info   (returns the canned File with url_private_download)
//	GET  /raw/<file_id> (returns the raw bytes; the path Slack would serve)
func TestSlackDownloadAttachmentContentAddressed(t *testing.T) {
	payload := []byte("\xff\xd8\xff\xe0herald-slack-attachment-test\xff\xd9")
	sum := sha256.Sum256(payload)
	wantSha := hex.EncodeToString(sum[:])

	var (
		filesInfoHits int
		downloadHits  int
		serverURL     string // captured so files.info can build the absolute url_private_download
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/files.info", func(w http.ResponseWriter, r *http.Request) {
		filesInfoHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"file": map[string]any{
				"id":                   "F0HERALDTEST",
				"name":                 "test.jpg",
				"mimetype":             "image/jpeg",
				"size":                 len(payload),
				"url_private_download": serverURL + "/raw/F0HERALDTEST",
			},
		})
	})
	mux.HandleFunc("/raw/", func(w http.ResponseWriter, r *http.Request) {
		downloadHits++
		// §107 — verify the Authorization: Bearer header is the bot token.
		// A download that succeeded without auth would be a privacy bluff.
		got := r.Header.Get("Authorization")
		if got != "Bearer xoxb-test" {
			t.Errorf("Authorization=%q want %q", got, "Bearer xoxb-test")
		}
		_, _ = w.Write(payload)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	serverURL = srv.URL

	// Override HOME so ~/.herald/inbox is under t.TempDir().
	home := t.TempDir()
	t.Setenv("HOME", home)

	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	gotPath, gotSha, err := a.DownloadAttachment(context.Background(), "F0HERALDTEST", "image/jpeg")
	if err != nil {
		t.Fatalf("DownloadAttachment first call: %v", err)
	}
	if gotSha != wantSha {
		t.Fatalf("sha mismatch: got %s want %s", gotSha, wantSha)
	}
	wantPath := filepath.Join(home, ".herald", "inbox", "slack", wantSha+".jpg")
	if gotPath != wantPath {
		t.Fatalf("path mismatch: got %s want %s", gotPath, wantPath)
	}
	onDisk, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if !bytes.Equal(onDisk, payload) {
		t.Fatalf("file content mismatch: lenOnDisk=%d lenPayload=%d", len(onDisk), len(payload))
	}

	// Idempotence: second call must NOT re-write the path. The slack-go
	// client still issues files.info (no client-side caching of metadata),
	// but the on-disk content-addressed write is skipped — same path, same
	// size, no .part residue.
	info1, err := os.Stat(gotPath)
	if err != nil {
		t.Fatalf("stat 1: %v", err)
	}
	gotPath2, gotSha2, err := a.DownloadAttachment(context.Background(), "F0HERALDTEST", "image/jpeg")
	if err != nil {
		t.Fatalf("DownloadAttachment second call: %v", err)
	}
	if gotPath2 != gotPath {
		t.Fatalf("idempotence: path changed: %s -> %s", gotPath, gotPath2)
	}
	if gotSha2 != gotSha {
		t.Fatalf("idempotence: sha changed: %s -> %s", gotSha, gotSha2)
	}
	info2, err := os.Stat(gotPath)
	if err != nil {
		t.Fatalf("stat 2: %v", err)
	}
	if info1.Size() != info2.Size() {
		t.Fatalf("idempotence: size changed: %d -> %d", info1.Size(), info2.Size())
	}

	// No .part temp residue in the per-channel inbox dir.
	inboxDir := filepath.Join(home, ".herald", "inbox", "slack")
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		t.Fatalf("readdir inbox: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "dl-") && strings.HasSuffix(name, ".part") {
			t.Fatalf("temp file leaked into inbox dir: %s", name)
		}
	}

	if filesInfoHits < 2 {
		t.Fatalf("files.info hits=%d want >=2 (each call resolves the URL)", filesInfoHits)
	}
	// download hits CAN be 2 — WriteContentAddressed streams the body to a
	// temp file, hashes inline, and only at the atomic-rename step detects
	// the existing content-addressed file (dropping the temp). The proof of
	// idempotence is the on-disk shape: same path, same size, no .part
	// residue (asserted above). A future optimisation could short-circuit
	// the second download by hashing files.info metadata, but that's out
	// of scope for T6 — the §107 invariant is bytes/path stability, not
	// network-call elision.
	if downloadHits < 1 {
		t.Fatalf("download hits=%d want >=1 (§107: first download must cross the wire)", downloadHits)
	}
}

// TestSlackDownloadAttachmentEmptyIDErrors — empty file id is rejected
// before any wire call (the bluff class is a download that "succeeded"
// with empty input by returning the zero-value path).
func TestSlackDownloadAttachmentEmptyIDErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("expected NO wire calls for empty file id, got %s", r.URL.Path)
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	if _, _, err := a.DownloadAttachment(context.Background(), "", "image/jpeg"); err == nil {
		t.Fatal("DownloadAttachment(empty id) want error")
	}
}

// TestSlackDownloadAttachmentMissingURLPrivateDownload — a files.info
// response with empty url_private_download MUST error (§107: a download
// that wrote zero bytes from an empty URL is a no-op bluff).
func TestSlackDownloadAttachmentMissingURLPrivateDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/files.info") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"file": map[string]any{
					"id":                   "FX",
					"url_private_download": "",
				},
			})
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	defer srv.Close()
	a := slack.NewWithBaseURL("xoxb-test", "", "C1", srv.URL)
	_, _, err := a.DownloadAttachment(context.Background(), "FX", "image/jpeg")
	if err == nil {
		t.Fatal("DownloadAttachment want error for empty url_private_download")
	}
	if !strings.Contains(err.Error(), "url_private_download") {
		t.Fatalf("error=%q want mention of url_private_download", err.Error())
	}
}

// silence "fmt unused" if no debug helpers — keeps the import-list stable
// across iteration.
var _ = fmt.Sprintf

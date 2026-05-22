// pherald listen — Wave 6 T10a (2026-05-22) — QA evidence journaling.
//
// When `pherald listen` is invoked with `--qa-out-dir <path>`, the runtime
// journals every bidirectional event of the inbound pipeline to
// `<path>/transcript.jsonl` as JSONL records with shape:
//
//	{"ts":"<RFC3339Nano UTC>","direction":"in|out","kind":"...","payload":{...}}
//
// Four kinds are emitted per inbound cycle:
//   - `tgram.message`   (direction=in)  — the parsed InboundEvent
//   - `cc.dispatch`     (direction=out) — the CodeRequest sent to Claude Code
//   - `cc.reply`        (direction=in)  — the parsed Reply (action, text)
//   - `tgram.send_reply`(direction=out) — the SendReply call (chatID, replyToID, text)
//
// Attachments referenced in inbound events are copied (NOT symlinked) under
// `<path>/attachments/<sha256>.<ext>`. Copy survives later cleanup of the
// content-addressed inbox and avoids OS-specific symlink quirks.
//
// §107.x anchor: the JSONL transcript is THE positive-runtime-evidence
// artefact that proves an end-user exchange actually happened (per the
// docs/qa/<run-id>/ mandate). Every line is fsync'd after write so SIGINT
// during a handle does not lose tail events.
//
// Implementation note (interception strategy — external wrapping, NOT
// invasive Config injection): the journal sits as decorators over the
// three existing interfaces (commons.InboundHandler, inbound.CodeDispatcher,
// inbound.TgramReplier). The production listen pipeline composes them via
// runListen's existing seams; no inbound package surface is widened.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// journal owns the transcript.jsonl handle + attachments directory and
// serialises writes under a mutex (the inbound pipeline is single-threaded
// per message today, but tgram.Subscribe's handler may go concurrent in a
// future iteration; keep the file write side safe by construction).
type journal struct {
	mu          sync.Mutex
	dir         string
	attachDir   string
	f           *os.File
	now         func() time.Time
	closed      bool
}

// journalRecord is the exact wire shape per the operator-locked plan
// (Wave 6 T10a). RFC3339Nano + UTC; payload is opaque JSON (the encoder
// handles any json.Marshaler contained inside).
type journalRecord struct {
	TS        string `json:"ts"`
	Direction string `json:"direction"`
	Kind      string `json:"kind"`
	Payload   any    `json:"payload"`
}

// newJournal creates dir + dir/attachments + opens dir/transcript.jsonl in
// append mode (so re-runs in the same dir concatenate rather than truncate).
// Returns a usable *journal or an error; the error variant guarantees no
// partial state (caller can fail fast).
func newJournal(dir string) (*journal, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("pherald listen: journal dir is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("pherald listen: resolve qa-out-dir: %w", err)
	}
	attachDir := filepath.Join(abs, "attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		return nil, fmt.Errorf("pherald listen: mkdir attachments: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(abs, "transcript.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("pherald listen: open transcript.jsonl: %w", err)
	}
	return &journal{
		dir:       abs,
		attachDir: attachDir,
		f:         f,
		now:       func() time.Time { return time.Now().UTC() },
	}, nil
}

// write emits one JSONL line, then calls Sync() so a SIGINT/SIGTERM mid-
// pipeline cannot lose tail events. Returns the encoding/IO error if any
// (callers log + continue — the JSONL write is observability, not the
// load-bearing inbound path).
func (j *journal) write(direction, kind string, payload any) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("pherald listen: journal already closed")
	}
	rec := journalRecord{
		TS:        j.now().Format(time.RFC3339Nano),
		Direction: direction,
		Kind:      kind,
		Payload:   payload,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("pherald listen: marshal journal record: %w", err)
	}
	if _, err := j.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("pherald listen: write journal record: %w", err)
	}
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("pherald listen: sync journal: %w", err)
	}
	return nil
}

// close flushes + releases the file handle. Idempotent.
func (j *journal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	return j.f.Close()
}

// copyAttachment copies an inbound attachment into <attachDir>/<sha256>.<ext>.
// Idempotent — if the destination already exists (same content-addressed
// path), the function is a no-op and returns the existing path. The
// extension is derived from filepath.Ext(att.Filename); the sha256 hex is
// carried by att.CID (per the convention established in tgram/subscribe.go,
// which uses CID as the inbound sha256 carrier).
//
// Returns the relative path of the written file (relative to the journal
// dir) for embedding into the JSONL payload, plus any IO error.
func (j *journal) copyAttachment(att commons.Attachment) (string, error) {
	if j == nil {
		return "", errors.New("pherald listen: journal is nil")
	}
	if att.CID == "" {
		return "", fmt.Errorf("pherald listen: attachment %q has no sha256 (CID empty)", att.Filename)
	}
	ext := filepath.Ext(att.Filename)
	if ext == "" {
		ext = ".bin"
	}
	dst := filepath.Join(j.attachDir, att.CID+ext)
	// Idempotent: if a file already lives at the sha256 path, the bytes
	// are equal by content-addressing — skip the copy.
	if _, err := os.Stat(dst); err == nil {
		rel, _ := filepath.Rel(j.dir, dst)
		return rel, nil
	}
	rc, err := att.Reader()
	if err != nil {
		return "", fmt.Errorf("pherald listen: open attachment %q: %w", att.Filename, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("pherald listen: create attachment dest %q: %w", dst, err)
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return "", fmt.Errorf("pherald listen: copy attachment to %q: %w", dst, err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return "", fmt.Errorf("pherald listen: sync attachment %q: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("pherald listen: close attachment %q: %w", dst, err)
	}
	rel, _ := filepath.Rel(j.dir, dst)
	return rel, nil
}

// journalingHandler is the outermost decorator: it emits the inbound
// `tgram.message` line, copies attachments into <qa-out-dir>/attachments/,
// then delegates to the real *inbound.Dispatcher. The CC dispatch + reply
// + outbound reply journaling is layered on the CodeDispatcher and
// TgramReplier wrappers (journalingCode / journalingReplier), composed by
// runListen when --qa-out-dir is set.
type journalingHandler struct {
	j     *journal
	inner commons.InboundHandler
}

// Handle implements commons.InboundHandler. It logs the inbound event
// (with attachment sha256s + relative paths) BEFORE invoking the wrapped
// handler, so the JSONL preserves causal order even if the inner handler
// errors mid-way.
func (h *journalingHandler) Handle(ctx context.Context, ev commons.InboundEvent) error {
	atts := make([]map[string]any, 0, len(ev.Attachments))
	for _, a := range ev.Attachments {
		rel, err := h.j.copyAttachment(a)
		entry := map[string]any{
			"filename":   a.Filename,
			"mime":       a.MIMEType,
			"size_bytes": a.SizeBytes,
			"sha256":     a.CID,
		}
		if err == nil {
			entry["copied_to"] = rel
		} else {
			entry["copy_error"] = err.Error()
		}
		atts = append(atts, entry)
	}
	chatID := ""
	if ev.Sender.ChannelUserID != "" {
		chatID = ev.Sender.ChannelUserID
	}
	// message_id may live in Raw (the tgram OnText/OnPhoto/... handlers
	// populate it).
	var msgID any
	if ev.Raw != nil {
		if v, ok := ev.Raw["message_id"]; ok {
			msgID = v
		}
	}
	payload := map[string]any{
		"event_id":    ev.EventID,
		"channel":     ev.Sender.Channel,
		"chat_id":     chatID,
		"message_id":  msgID,
		"text":        ev.Body.Plain,
		"attachments": atts,
	}
	if err := h.j.write("in", "tgram.message", payload); err != nil {
		// Log-and-continue — observability MUST NOT block the inbound
		// pipeline. The error is reported to stderr via fmt.Fprintln so
		// the operator can see it in pherald-listen.log.
		fmt.Fprintln(os.Stderr, "journal: write tgram.message:", err)
	}
	return h.inner.Handle(ctx, ev)
}

// journalingCode wraps inbound.CodeDispatcher to capture `cc.dispatch`
// (request out to Claude) + `cc.reply` (raw stdout back from Claude).
// Both events are journaled even if the underlying Dispatch errors —
// the operator needs the trace either way.
type journalingCode struct {
	j     *journal
	inner inbound.CodeDispatcher
}

func (c *journalingCode) Dispatch(ctx context.Context, req inbound.CodeRequest) (inbound.CodeResponse, error) {
	atts := make([]map[string]any, 0, len(req.Attachments))
	for _, a := range req.Attachments {
		atts = append(atts, map[string]any{
			"filename":   a.Filename,
			"mime":       a.MIMEType,
			"size_bytes": a.SizeBytes,
			"sha256":     a.CID,
		})
	}
	if err := c.j.write("out", "cc.dispatch", map[string]any{
		"inbound_id":     req.InboundID,
		"sender":         req.Sender,
		"channel":        string(req.Channel),
		"user_message":   req.UserMessage,
		"attachments":    atts,
		"classification": req.Classification,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "journal: write cc.dispatch:", err)
	}
	resp, err := c.inner.Dispatch(ctx, req)
	replyPayload := map[string]any{
		"stdout_bytes": len(resp.Stdout),
		"stdout":       string(resp.Stdout),
	}
	if err != nil {
		replyPayload["error"] = err.Error()
	}
	// Try to extract action + text from the marker-prefixed stdout for a
	// richer journal entry; tolerant of parse failure (raw stdout is
	// always preserved above).
	if r, perr := inbound.ParseReply(resp.Stdout); perr == nil && r != nil {
		replyPayload["action"] = r.Action
		replyPayload["text"] = r.Text
	}
	if werr := c.j.write("in", "cc.reply", replyPayload); werr != nil {
		fmt.Fprintln(os.Stderr, "journal: write cc.reply:", werr)
	}
	return resp, err
}

// journalingReplier wraps inbound.TgramReplier to capture every outbound
// SendReply (the canonical tgram.send_reply event with reply_to_message_id).
type journalingReplier struct {
	j     *journal
	inner inbound.TgramReplier
}

func (r *journalingReplier) SendReply(ctx context.Context, chatID int64, text string, replyToID int, attachments []commons.Attachment) (int, error) {
	atts := make([]map[string]any, 0, len(attachments))
	for _, a := range attachments {
		atts = append(atts, map[string]any{
			"filename":   a.Filename,
			"mime":       a.MIMEType,
			"size_bytes": a.SizeBytes,
			"sha256":     a.CID,
		})
	}
	msgID, err := r.inner.SendReply(ctx, chatID, text, replyToID, attachments)
	payload := map[string]any{
		"chat_id":             chatID,
		"reply_to_message_id": replyToID,
		"text":                text,
		"attachments":         atts,
		"sent_message_id":     msgID,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	if werr := r.j.write("out", "tgram.send_reply", payload); werr != nil {
		fmt.Fprintln(os.Stderr, "journal: write tgram.send_reply:", werr)
	}
	return msgID, err
}

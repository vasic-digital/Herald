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
	if channel == "" {
		return "", fmt.Errorf("channels.InboxDir: empty channel")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("channels.InboxDir: resolve home: %w", err)
	}
	dir := filepath.Join(home, ".herald", "inbox", channel)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("channels.InboxDir: mkdir %s: %w", dir, err)
	}
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
	if err != nil {
		return "", "", err
	}
	tmp, err := os.CreateTemp(dir, "dl-*.part")
	if err != nil {
		return "", "", fmt.Errorf("channels.WriteContentAddressed: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	hasher := sha256.New()
	// io.MultiWriter streams the body to disk + sha256 in one pass; no
	// intermediate buffering of the full payload.
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), r); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", "", fmt.Errorf("channels.WriteContentAddressed: stream: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", "", fmt.Errorf("channels.WriteContentAddressed: close temp: %w", err)
	}
	sumHex := hex.EncodeToString(hasher.Sum(nil))
	finalPath := filepath.Join(dir, sumHex+"."+MimeToExt(mime))
	// Idempotence: if the content-addressed file already exists, the bytes are
	// byte-equal by construction (same sha256 → same content). Drop the temp
	// file and return the existing path unchanged.
	if _, statErr := os.Stat(finalPath); statErr == nil {
		cleanup()
		return finalPath, sumHex, nil // idempotent: content == proof
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return "", "", fmt.Errorf("channels.WriteContentAddressed: rename: %w", err)
	}
	return finalPath, sumHex, nil
}

// MimeToExt returns the canonical filename extension for the small set of MIME
// types messenger subscribers actually send (photo/document/voice). Unknown
// MIMEs fall back to "bin" so the on-disk file is always recoverable. Promoted
// VERBATIM from the Wave 6 tgram-private mimeToExt switch (commons_messaging/
// channels/tgram/attachments.go) so every channel adapter shares one map.
func MimeToExt(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "video/mp4":
		return "mp4"
	case "audio/ogg", "audio/opus":
		return "ogg"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "application/pdf":
		return "pdf"
	case "text/plain":
		return "txt"
	default:
		return "bin"
	}
}

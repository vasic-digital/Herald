package tgram

import (
	"context"
	"errors"
	"fmt"

	telebot "gopkg.in/telebot.v3"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// DownloadAttachment fetches the Telegram-hosted file identified by
// fileID, streams it into ~/.herald/inbox/tgram/<sha256>.<ext> while
// computing the sha256 inline, and returns (finalPath, sha256Hex, error).
//
// As of Wave 7 T3 (HRD-112) the stream-hash-rename body lives in the shared
// channels.WriteContentAddressed helper, and the storage root is the
// per-channel subdir ~/.herald/inbox/tgram/ (channels.InboxDir). This file
// now only does the Telegram-specific part — fetch the reader via bot.File —
// then hands the reader to the channel-agnostic helper.
//
// Content-addressing guarantee: the final path is sha256-derived, so the
// SAME bytes always land at the SAME path regardless of fileID. A second
// call for the same content is a no-op (the existing file IS the proof
// of presence) — this is critical so pherald's getUpdates loop doesn't
// burn Bot API quota re-downloading every poll cycle.
//
// Streaming guarantee: the payload is never fully buffered in memory.
// channels.WriteContentAddressed uses io.MultiWriter(tmpFile, hasher) so the
// body flows directly to disk while the sha256 is computed inline; the final
// atomic os.Rename guarantees that a partially-written file is never visible
// at the content-addressed path.
//
// §107 anti-bluff anchor (CLAUDE.md §107.x). A handler that "succeeds"
// but writes zero bytes / writes to a fixed path / re-writes on every
// duplicate would pass type checks. The companion test
// TestDownloadAttachmentContentAddressed pins all three failure modes by
// asserting (a) the final path is exactly home/.herald/inbox/tgram/<sha>.<ext>,
// (b) the on-disk bytes equal the canned payload, and (c) a duplicate
// download returns the same path with the same size and leaves no .part
// temp file behind.
//
// mime → extension mapping is the shared channels.MimeToExt — intentionally
// narrow, the canonical extensions Telegram subscribers actually send
// (photo/document/voice). Unknown MIMEs fall back to .bin.
func DownloadAttachment(ctx context.Context, bot *telebot.Bot, fileID, mime string) (string, string, error) {
	if bot == nil {
		return "", "", errors.New("tgram.DownloadAttachment: nil bot")
	}
	if fileID == "" {
		return "", "", errors.New("tgram.DownloadAttachment: empty fileID")
	}

	// telebot.v3 API:
	//   - b.FileByID(fileID) (File, error) — populates FilePath via getFile
	//   - b.File(&File{FileID: ...}) (io.ReadCloser, error) — fetches bytes
	// Verified by reading submodules/telebot/bot.go:967 + bot.go:1011.
	rc, err := bot.File(&telebot.File{FileID: fileID})
	if err != nil {
		return "", "", fmt.Errorf("tgram.DownloadAttachment: bot.File(%s): %w", fileID, err)
	}

	// channels.WriteContentAddressed closes rc, hashes inline, atomic-renames
	// into ~/.herald/inbox/tgram/, and is idempotent on duplicate content.
	path, sum, werr := channels.WriteContentAddressed(string(commons.ChannelTelegram), mime, rc)
	if werr != nil {
		return "", "", fmt.Errorf("tgram.DownloadAttachment: %w", werr)
	}
	// ctx is reserved for future cancellation support; telebot.v3's
	// Bot.File() does not currently accept a context. A future HRD will
	// wrap the underlying http.Client to honour ctx.Done(). Touching ctx
	// here keeps the parameter from being silently strippable later.
	_ = ctx
	return path, sum, nil
}


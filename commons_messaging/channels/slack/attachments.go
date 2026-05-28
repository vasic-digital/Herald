package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
)

// DownloadAttachment fetches the Slack-hosted file identified by externalID
// (the file_id returned by files.info / message.files[].id), streams it
// into ~/.herald/inbox/slack/<sha256>.<ext> via channels.WriteContentAddressed
// while computing the sha256 inline, and returns (finalPath, sha256Hex, error).
//
// Wire steps:
//
//  1. files.info(externalID) → resolve url_private_download.
//  2. GET url_private_download with Authorization: Bearer <bot_token>.
//  3. Stream the body through channels.WriteContentAddressed (writes to
//     a temp file + hashes inline + atomic-renames into the per-channel
//     inbox subdir; closes the response body).
//
// Content-addressing + idempotence: identical content always lands at the
// same path regardless of file_id; a duplicate download is a no-op (the
// existing on-disk file IS the proof of presence). This is critical so
// pherald's Socket Mode loop does not burn Slack API quota re-downloading
// the same shared file on every restart.
//
// §107 anti-bluff anchor: a handler that "succeeds" but writes zero bytes
// / writes to a fixed path / re-writes on every duplicate would pass type
// checks. The companion test TestDownloadAttachmentContentAddressed pins
// all three failure modes by asserting (a) exact path shape, (b) on-disk
// byte equality, (c) no .part residue.
//
// mime override: if the caller passes mime == "" the method falls back
// to info.Mimetype from files.info; mime != "" wins (callers who already
// know the type — e.g. Subscribe routing — pass it through).
func (a *Adapter) DownloadAttachment(ctx context.Context, externalID, mime string) (string, string, error) {
	if externalID == "" {
		return "", "", errors.New("slack.DownloadAttachment: empty file id")
	}
	info, _, _, err := a.api.GetFileInfoContext(ctx, externalID, 0, 0)
	if err != nil {
		return "", "", fmt.Errorf("slack.DownloadAttachment: files.info(%s): %w", externalID, err)
	}
	if info == nil || info.URLPrivateDownload == "" {
		return "", "", fmt.Errorf("slack.DownloadAttachment: file %s has no url_private_download", externalID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URLPrivateDownload, nil)
	if err != nil {
		return "", "", fmt.Errorf("slack.DownloadAttachment: build GET %s: %w", info.URLPrivateDownload, err)
	}
	req.Header.Set("Authorization", "Bearer "+a.botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("slack.DownloadAttachment: GET url_private_download: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return "", "", fmt.Errorf("slack.DownloadAttachment: url_private_download status %d", resp.StatusCode)
	}
	effMime := mime
	if effMime == "" {
		effMime = info.Mimetype
	}
	// channels.WriteContentAddressed closes resp.Body, hashes inline,
	// atomic-renames into ~/.herald/inbox/slack/, and is idempotent on
	// duplicate content.
	path, sum, werr := channels.WriteContentAddressed(string(commons.ChannelSlack), effMime, resp.Body)
	if werr != nil {
		return "", "", fmt.Errorf("slack.DownloadAttachment: %w", werr)
	}
	return path, sum, nil
}

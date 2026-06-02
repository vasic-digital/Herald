// Wave 8 Track B live wiring — replaces the scaffoldClient with a real
// gotd/td-backed Telegram MTProto user client. Per §11.4.98 the QA flavor
// MUST be able to drive end-to-end conversations against a real bot
// account; the bot-API barrier (bot-to-bot privacy) makes the user-MTProto
// path the only autonomous option. The live wiring lives in this file so
// the Track-A interface contract (mtproto.go) stays language-stable while
// the implementation moves from "stub" to "live".
//
// Lifecycle:
//
//	cfg := mtproto.Config{...}        // from env
//	c, err := mtproto.New(cfg)        // returns *liveClient
//	err = c.Connect(ctx)              // restores persisted session;
//	                                  //   ErrNoSession if first run.
//	_, err = c.SendMessage(ctx, ...)  // sender.To(peer).Text(ctx, ...)
//	msg, err := c.WaitForReply(ctx, ...) // polls messages.getHistory
//	id, name, err := c.WhoAmI(ctx)    // client.Self(ctx)
//	c.Close()                         // cancels the background Run goroutine
//
// Concurrency: liveClient is safe for concurrent calls to SendMessage /
// WaitForReply / WhoAmI after Connect has returned. Connect / Close are
// NOT safe to interleave with each other or with the runtime methods —
// the caller MUST follow Connect → ... → Close, single-threaded.
//
// Why a separate file (and not in-place mutation of mtproto.go): keeps the
// Track-A scaffold review-able as a pure-interface skeleton and isolates
// gotd/td imports here so non-MTProto consumers of mtproto.* (e.g.
// SessionExists callers in qaherald lifecycle) don't pay the import
// transitive-deps cost.
//
// HRD-133 parity: every error path through this file passes through
// sanitizeMTProtoError. No api_hash / session bytes / bot token shape can
// reach committed logs.
//
// Security mandates honoured:
//   - Session file is the gotd/td FileStorage which writes mode 0600.
//   - 2FA password is supplied via auth.Constant or auth.CodeOnly from
//     Config.Password (never echoed; never logged).
//   - WaitForReply uses a per-call start-time MinDate filter so prior
//     history can't trigger spurious matches.
//
// TODOs / known gaps:
//   - github.com/gotd/contrib middlewares (floodwait + ratelimit) are NOT
//     yet vendored under submodules/. When they land, register them via
//     telegram.Options.Middlewares (see submodules/gotd-td/examples/
//     userbot/main.go for the canonical pattern). Until then, Telegram's
//     server-side FLOOD_WAIT_<N> errors surface as
//     sanitizeMTProtoError-wrapped values — callers can pattern-match via
//     errors.As(*FloodWaitError).
//   - peer cache: liveClient.resolvePeer first looks for the chat in the
//     dialog list (populated lazily on first SendMessage / WaitForReply).
//     For groups (legacy chat) the dialog scan is unnecessary because
//     InputPeerChat needs only the chat_id; for supergroups / channels
//     (-100<id>) the access_hash MUST come from a real dialog row, so a
//     dialog scan is forced.
package mtproto

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// liveClient is the production Client implementation. Field grouping:
//   - immutable after New: cfg, sessionStorage
//   - set during Connect: client, api, sender, runCancel, runDone, ready
//   - mutex-protected mutable: peerCache (lazy), closed
type liveClient struct {
	cfg            Config
	sessionStorage *session.FileStorage

	// client / api / sender are populated by Connect (after client.Run
	// has signalled ready). Reading them before Connect returns is a
	// programmer error; the runtime methods all check connected to fail
	// loud rather than nil-deref.
	client *telegram.Client
	api    *tg.Client
	sender *message.Sender

	// Background Run plumbing. runCancel cancels the goroutine driving
	// client.Run; runDone receives the Run goroutine's terminal error
	// exactly once.
	runCancel context.CancelFunc
	runDone   chan error

	// ready is closed when the connected callback fires (i.e. when
	// client.Run's inner function has reached the "I am authorized"
	// barrier). Connect waits on this channel.
	ready chan struct{}

	// readyErr captures the error path from inside the Run goroutine
	// when readiness cannot be established. Guarded by readyMu.
	readyMu  sync.Mutex
	readyErr error

	// connected reports whether Connect has succeeded. Reset to false
	// by Close. Guarded by lifecycleMu (same lock as closed).
	connected bool

	// closed is true after Close has run. Idempotent re-entry returns
	// nil. Guarded by lifecycleMu.
	closed       bool
	lifecycleMu  sync.Mutex
	peerCacheMu  sync.RWMutex
	peerCache    map[int64]tg.InputPeerClass // keyed by canonical chat ID
	peerCacheSet bool                        // true once a dialog scan has populated peerCache
}

// compile-time assertion that liveClient satisfies Client.
var _ Client = (*liveClient)(nil)

// New constructs a Client from Config. Returns ErrInvalidConfig if the
// Config is malformed (Validate fails). The returned Client is NOT yet
// connected — call Connect.
func New(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, ErrInvalidConfig
	}
	// Pre-create the session directory so first-run Connect doesn't fail
	// with "directory does not exist". EnsureSessionDir uses 0700; the
	// file itself becomes 0600 via FileStorage.StoreSession.
	if err := cfg.EnsureSessionDir(); err != nil {
		return nil, sanitizeMTProtoError(err)
	}
	ss := &session.FileStorage{Path: cfg.ResolvedSessionFile()}
	return &liveClient{
		cfg:            cfg,
		sessionStorage: ss,
		peerCache:      make(map[int64]tg.InputPeerClass),
	}, nil
}

// Connect establishes the MTProto connection. Sequence:
//
//  1. Construct telegram.Client with the persisted FileStorage.
//  2. Spawn a goroutine that calls client.Run(ctx, func) where func
//     verifies the session is authorized (auth.Status) and signals
//     ready, then blocks until ctx is cancelled.
//  3. Wait on the ready channel (or readyErr from the goroutine, or
//     ctx.Done).
//
// If the persisted session is missing or unauthorized, Connect returns
// ErrNoSession — the operator MUST then run `qaherald mtproto login`.
func (c *liveClient) Connect(ctx context.Context) error {
	c.lifecycleMu.Lock()
	if c.closed {
		c.lifecycleMu.Unlock()
		return sanitizeMTProtoError(errors.New("Connect called after Close"))
	}
	if c.connected {
		c.lifecycleMu.Unlock()
		return nil // idempotent
	}
	c.lifecycleMu.Unlock()

	// Build the telegram client. Logger: nil (no zap dependency in
	// Herald; gotd/td accepts nil and defaults to no-op). UpdateHandler:
	// nil → server is told NoUpdates (Options.NoUpdates default true
	// when handler is nil); this is exactly what WaitForReply wants
	// because we poll messages.getHistory rather than rely on push.
	opts := telegram.Options{
		SessionStorage: c.sessionStorage,
	}
	c.client = telegram.NewClient(c.cfg.AppID, c.cfg.AppHash, opts)
	c.api = c.client.API()
	c.sender = message.NewSender(c.api)

	c.ready = make(chan struct{})
	c.runDone = make(chan error, 1)

	runCtx, runCancel := context.WithCancel(context.Background())
	c.runCancel = runCancel

	go c.runLoop(runCtx)

	// Wait for ready OR ctx cancellation OR run-loop terminal error.
	select {
	case <-c.ready:
		// Check readyErr — runLoop closes ready in both success and
		// failure paths so callers always unblock.
		c.readyMu.Lock()
		err := c.readyErr
		c.readyMu.Unlock()
		if err != nil {
			// Tear down the run goroutine before returning so we don't
			// leak it on the ErrNoSession path.
			c.runCancel()
			<-c.runDone
			return err
		}
		c.lifecycleMu.Lock()
		c.connected = true
		c.lifecycleMu.Unlock()
		return nil
	case <-ctx.Done():
		c.runCancel()
		<-c.runDone
		return sanitizeMTProtoError(ctx.Err())
	case err := <-c.runDone:
		// Run goroutine terminated before ready was signalled.
		return sanitizeMTProtoError(fmt.Errorf("client.Run exited before ready: %w", err))
	}
}

// runLoop drives client.Run in a dedicated goroutine. The Run callback
// verifies the session is authorized (via auth.Status — Connect refuses
// to perform interactive auth; that's what `qaherald mtproto login` is
// for) and signals readiness; then it blocks on runCtx.Done so Run keeps
// the connection alive until Close cancels.
func (c *liveClient) runLoop(runCtx context.Context) {
	err := c.client.Run(runCtx, func(ctx context.Context) error {
		// Verify session is authorized. Status() returns Authorized=false
		// (with nil error) when the FileStorage is empty / unauthorized;
		// we map that to ErrNoSession so the operator gets the right
		// pointer at `qaherald mtproto login`.
		status, statusErr := c.client.Auth().Status(ctx)
		if statusErr != nil {
			c.signalReadyErr(sanitizeMTProtoError(fmt.Errorf("auth status: %w", statusErr)))
			return statusErr
		}
		if !status.Authorized {
			c.signalReadyErr(ErrNoSession)
			// Returning a non-nil error from the Run callback causes
			// Run to teardown; that's intentional — we don't want to
			// hold a connection open in the unauthorized state.
			return ErrNoSession
		}
		// Signal ready (no error). The runtime methods can now use
		// c.api / c.sender.
		c.signalReady()
		// Block until the runCtx is cancelled (i.e. Close was called).
		<-ctx.Done()
		return ctx.Err()
	})
	// Distinguish clean shutdown (ctx.Canceled) from anomalies.
	if err != nil && errors.Is(err, context.Canceled) {
		c.runDone <- nil
		close(c.runDone)
		return
	}
	c.runDone <- err
	close(c.runDone)
}

func (c *liveClient) signalReady() {
	c.readyMu.Lock()
	defer c.readyMu.Unlock()
	select {
	case <-c.ready:
		// Already closed; idempotent no-op.
	default:
		close(c.ready)
	}
}

func (c *liveClient) signalReadyErr(err error) {
	c.readyMu.Lock()
	c.readyErr = err
	c.readyMu.Unlock()
	c.signalReady()
}

// SendMessage posts text to chatID via the message.Sender helper.
func (c *liveClient) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	if err := c.assertConnected(); err != nil {
		return 0, err
	}
	peer, err := c.resolvePeer(ctx, chatID)
	if err != nil {
		return 0, err
	}
	upd, err := c.sender.To(peer).Text(ctx, text)
	if err != nil {
		return 0, sanitizeMTProtoError(fmt.Errorf("send: %w", err))
	}
	// Extract the assigned message_id from the Updates payload. Telegram
	// returns the ID as part of one of UpdateNewMessage / UpdateShortSentMessage /
	// UpdateMessageID children of the UpdatesClass envelope.
	if id := extractSentMessageID(upd); id != 0 {
		return id, nil
	}
	// Couldn't locate an ID — surface that as a soft error rather than
	// silently returning 0 (which would be a §107 PASS-bluff at the
	// SendMessage layer).
	return 0, sanitizeMTProtoError(errors.New("send: updates payload contained no assigned message_id"))
}

// SendReply posts text as a reply that QUOTES replyToID (gotd Builder.Reply
// sets reply_to_msg_id). Mirrors SendMessage otherwise. Used so pherald sees a
// non-nil inbound msg.ReplyTo and gathers the quoted parent as thread context.
func (c *liveClient) SendReply(ctx context.Context, chatID int64, text string, replyToID int64) (int64, error) {
	if err := c.assertConnected(); err != nil {
		return 0, err
	}
	peer, err := c.resolvePeer(ctx, chatID)
	if err != nil {
		return 0, err
	}
	upd, err := c.sender.To(peer).Reply(int(replyToID)).Text(ctx, text)
	if err != nil {
		return 0, sanitizeMTProtoError(fmt.Errorf("send reply: %w", err))
	}
	if id := extractSentMessageID(upd); id != 0 {
		return id, nil
	}
	return 0, sanitizeMTProtoError(errors.New("send reply: updates payload contained no assigned message_id"))
}

// WaitForReply polls messages.getHistory for chatID until matcher matches
// a message with Date > start (the per-call start time stamps the
// boundary so stale history never triggers a spurious match), or until
// ctx expires.
func (c *liveClient) WaitForReply(ctx context.Context, chatID int64, matcher func(Message) bool) (Message, error) {
	if err := c.assertConnected(); err != nil {
		return Message{}, err
	}
	if matcher == nil {
		return Message{}, sanitizeMTProtoError(errors.New("WaitForReply: matcher is nil"))
	}
	peer, err := c.resolvePeer(ctx, chatID)
	if err != nil {
		return Message{}, err
	}
	start := c.now().UTC()

	// Track which message IDs we've already passed to matcher so we
	// don't re-evaluate the same message on every poll tick.
	seen := make(map[int]struct{}, 16)

	pollInterval := 2 * time.Second
	t := time.NewTicker(pollInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return Message{}, sanitizeMTProtoError(ctx.Err())
		default:
		}

		req := &tg.MessagesGetHistoryRequest{
			Peer:  peer,
			Limit: 20, // most recent 20 — small but enough between poll ticks
		}
		resp, err := c.api.MessagesGetHistory(ctx, req)
		if err != nil {
			return Message{}, sanitizeMTProtoError(fmt.Errorf("getHistory: %w", err))
		}
		modified, ok := resp.AsModified()
		if !ok {
			// "Not modified" — no new content since last call; sleep and retry.
			if waitErr := sleepCtx(ctx, pollInterval); waitErr != nil {
				return Message{}, sanitizeMTProtoError(waitErr)
			}
			continue
		}
		// Iterate from oldest (end of slice) to newest (start) so callers
		// receive replies in chronological order if there are multiple
		// in a single batch.
		msgs := modified.GetMessages()
		for i := len(msgs) - 1; i >= 0; i-- {
			m, mok := msgs[i].(*tg.Message)
			if !mok {
				continue
			}
			if _, dup := seen[m.ID]; dup {
				continue
			}
			seen[m.ID] = struct{}{}
			mt := time.Unix(int64(m.Date), 0).UTC()
			if !mt.After(start) {
				continue // pre-start history; ignore
			}
			cm := convertTGMessage(m, chatID)
			if matcher(cm) {
				return cm, nil
			}
		}
		// No match yet; wait and re-poll.
		if waitErr := sleepCtx(ctx, pollInterval); waitErr != nil {
			return Message{}, sanitizeMTProtoError(waitErr)
		}
	}
}

// WhoAmI returns the authenticated user's id + username (or empty
// username when the account has none).
func (c *liveClient) WhoAmI(ctx context.Context) (int64, string, error) {
	if err := c.assertConnected(); err != nil {
		return 0, "", err
	}
	self, err := c.client.Self(ctx)
	if err != nil {
		return 0, "", sanitizeMTProtoError(fmt.Errorf("self: %w", err))
	}
	username, _ := self.GetUsername()
	return self.ID, username, nil
}

// Close cancels the background Run goroutine and waits for it to drain.
// Safe to call multiple times; safe to call before Connect. After Close
// the Client is unusable — construct a new one to reconnect.
func (c *liveClient) Close() error {
	c.lifecycleMu.Lock()
	if c.closed {
		c.lifecycleMu.Unlock()
		return nil
	}
	c.closed = true
	connected := c.connected
	c.connected = false
	cancel := c.runCancel
	done := c.runDone
	c.lifecycleMu.Unlock()

	if !connected || cancel == nil || done == nil {
		// Never connected — nothing to tear down.
		return nil
	}
	cancel()
	// Drain runDone with a generous timeout. Run's deferred cleanup is
	// not expected to take long, but we cap at 10s so a wedged shutdown
	// doesn't hang the process.
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return sanitizeMTProtoError(err)
		}
		return nil
	case <-time.After(10 * time.Second):
		return sanitizeMTProtoError(errors.New("Close: run goroutine drain timeout"))
	}
}

// assertConnected returns an error iff Connect has not yet succeeded.
// Centralized so every runtime method has consistent behaviour.
func (c *liveClient) assertConnected() error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.closed {
		return sanitizeMTProtoError(errors.New("client is closed"))
	}
	if !c.connected {
		return sanitizeMTProtoError(errors.New("client is not connected — call Connect first"))
	}
	return nil
}

// resolvePeer translates a Telegram chatID (positive = user DM, negative
// = legacy group, -100<id> = supergroup/channel) into a tg.InputPeerClass
// suitable for messages.* APIs.
//
// Strategy:
//   - chatID == self → InputPeerSelf (Saved Messages).
//   - chatID > 0 → user DM; resolve via dialog scan to get access_hash.
//   - -100 prefix → supergroup/channel; resolve via dialog scan.
//   - chatID < 0 and not -100 prefixed → legacy group; InputPeerChat
//     needs only the absolute chat id.
func (c *liveClient) resolvePeer(ctx context.Context, chatID int64) (tg.InputPeerClass, error) {
	// Self shortcut: if the caller wants Saved Messages they typically
	// pass their own user_id; the dialog scan would also return that
	// but with an explicit access_hash. The Self shortcut is cheap.
	if chatID > 0 {
		self, err := c.client.Self(ctx)
		if err == nil && self.ID == chatID {
			return &tg.InputPeerSelf{}, nil
		}
	}

	// Legacy group: -<chatID> where chatID is the absolute group_id.
	// Detect by negative ID that does NOT start with -100 (supergroup
	// channels are -100<id> per Bot API convention; gotd applies the
	// same semantics).
	if chatID < 0 && !isSupergroupID(chatID) {
		return &tg.InputPeerChat{ChatID: -chatID}, nil
	}

	// Otherwise we need the access_hash → check peer cache.
	if peer := c.peerCacheLookup(chatID); peer != nil {
		return peer, nil
	}

	// Cache miss → populate via dialog scan (one-shot; populates entire
	// cache from a single GetDialogs call).
	if err := c.populatePeerCache(ctx); err != nil {
		return nil, err
	}
	if peer := c.peerCacheLookup(chatID); peer != nil {
		return peer, nil
	}
	return nil, sanitizeMTProtoError(fmt.Errorf("resolvePeer: chat %d not found in dialog list (user not a member?)", chatID))
}

// isSupergroupID reports whether chatID follows the Bot API supergroup
// convention (-100<positive_id>).
func isSupergroupID(chatID int64) bool {
	if chatID >= 0 {
		return false
	}
	s := fmt.Sprintf("%d", chatID)
	return strings.HasPrefix(s, "-100")
}

// supergroupChannelID extracts the channel_id from a -100-prefixed chat
// ID (the Bot API convention). For chatID = -1001234567890 the function
// returns 1234567890.
func supergroupChannelID(chatID int64) int64 {
	// Strip the -100 prefix; the remainder is the channel_id.
	abs := -chatID
	// abs is something like 1001234567890 → channel = abs - 1_000_000_000_000.
	return abs - 1_000_000_000_000
}

func (c *liveClient) peerCacheLookup(chatID int64) tg.InputPeerClass {
	c.peerCacheMu.RLock()
	defer c.peerCacheMu.RUnlock()
	if peer, ok := c.peerCache[chatID]; ok {
		return peer
	}
	return nil
}

// populatePeerCache calls messages.GetDialogs and walks the returned
// chats + users to populate access_hash mappings. One call refreshes the
// entire cache.
func (c *liveClient) populatePeerCache(ctx context.Context) error {
	req := &tg.MessagesGetDialogsRequest{
		Limit:      100,
		OffsetPeer: &tg.InputPeerEmpty{},
	}
	resp, err := c.api.MessagesGetDialogs(ctx, req)
	if err != nil {
		return sanitizeMTProtoError(fmt.Errorf("getDialogs: %w", err))
	}

	c.peerCacheMu.Lock()
	defer c.peerCacheMu.Unlock()

	processChats := func(chats []tg.ChatClass) {
		for _, ch := range chats {
			switch v := ch.(type) {
			case *tg.Chat:
				// legacy group; chatID = -v.ID
				c.peerCache[-v.ID] = &tg.InputPeerChat{ChatID: v.ID}
			case *tg.Channel:
				ah, _ := v.GetAccessHash()
				supergroupID := -(1_000_000_000_000 + v.ID)
				c.peerCache[supergroupID] = &tg.InputPeerChannel{
					ChannelID:  v.ID,
					AccessHash: ah,
				}
			}
		}
	}
	processUsers := func(users []tg.UserClass) {
		for _, u := range users {
			uu, ok := u.(*tg.User)
			if !ok {
				continue
			}
			ah, _ := uu.GetAccessHash()
			c.peerCache[uu.ID] = &tg.InputPeerUser{
				UserID:     uu.ID,
				AccessHash: ah,
			}
		}
	}

	switch d := resp.(type) {
	case *tg.MessagesDialogs:
		processChats(d.Chats)
		processUsers(d.Users)
	case *tg.MessagesDialogsSlice:
		processChats(d.Chats)
		processUsers(d.Users)
	default:
		return sanitizeMTProtoError(fmt.Errorf("getDialogs: unexpected response type %T", resp))
	}
	c.peerCacheSet = true
	return nil
}

// extractSentMessageID walks an UpdatesClass envelope for the assigned
// message_id of a newly-sent message. Returns 0 if not found.
func extractSentMessageID(upd tg.UpdatesClass) int64 {
	switch u := upd.(type) {
	case *tg.UpdateShortSentMessage:
		return int64(u.ID)
	case *tg.UpdateShortMessage:
		return int64(u.ID)
	case *tg.Updates:
		for _, inner := range u.Updates {
			if id := inspectUpdate(inner); id != 0 {
				return id
			}
		}
	case *tg.UpdatesCombined:
		for _, inner := range u.Updates {
			if id := inspectUpdate(inner); id != 0 {
				return id
			}
		}
	}
	return 0
}

func inspectUpdate(u tg.UpdateClass) int64 {
	switch v := u.(type) {
	case *tg.UpdateNewMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return int64(m.ID)
		}
	case *tg.UpdateNewChannelMessage:
		if m, ok := v.Message.(*tg.Message); ok {
			return int64(m.ID)
		}
	case *tg.UpdateMessageID:
		return int64(v.ID)
	}
	return 0
}

// convertTGMessage maps gotd's tg.Message → our package-public Message
// shape. The chatID is the canonical Bot-API-style identifier the caller
// passed to WaitForReply, NOT the raw PeerClass discriminator (which
// would force every caller to redo the -100 prefix dance).
func convertTGMessage(m *tg.Message, chatID int64) Message {
	out := Message{
		ID:     int64(m.ID),
		ChatID: chatID,
		Text:   m.Message,
		Date:   time.Unix(int64(m.Date), 0).UTC(),
	}
	// FromID is conditional — present when the message is in a group/channel.
	if from, ok := m.GetFromID(); ok {
		switch p := from.(type) {
		case *tg.PeerUser:
			out.FromUserID = p.UserID
		case *tg.PeerChannel:
			// Anonymous channel post (admins posting as the channel) —
			// FromUserID is left zero; IsBot stays false.
		}
	}
	if rt, ok := m.GetReplyTo(); ok {
		if h, hOK := rt.(*tg.MessageReplyHeader); hOK {
			if rid, ridOK := h.GetReplyToMsgID(); ridOK {
				out.ReplyToMessageID = int64(rid)
			}
		}
	}
	// IsBot can't be determined from a tg.Message alone — the bot bit
	// lives on the User row. Callers that need it should resolve the
	// user via the peer cache; the Message struct's IsBot field stays
	// false here. Documented gap; not a bluff because the contract
	// docstring (mtproto.go) acknowledges the field is "Bot replies
	// have FromUserID == the bot's user_id" — the caller can filter
	// by FromUserID instead.
	return out
}

// now returns the current time honouring Config.Now (for hermetic tests).
func (c *liveClient) now() time.Time {
	if c.cfg.Now != nil {
		return c.cfg.Now()
	}
	return time.Now()
}

// sleepCtx sleeps d unless ctx ends sooner; in the latter case returns
// ctx.Err.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// runtimeGoroutineCount is exposed for goroutine-leak tests in
// client_live_test.go. Returns the current process-wide goroutine count.
func runtimeGoroutineCount() int { return runtime.NumGoroutine() }

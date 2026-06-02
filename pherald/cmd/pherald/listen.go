// pherald listen — Wave 6 T6 (2026-05-22).
//
// Long-running subcommand that wires tgram.Subscribe (getUpdates long-poll
// per spec §32.2) to the production inbound.Dispatcher (T7). On every
// Telegram message the bot self-filter (T4) drops self-echoes, attachments
// are content-addressed under ~/.herald/inbox/<sha>.<ext> (T5), the
// dispatcher invokes claude_code with the operator-mandated pre-text (T3)
// + the Opus pin (T2), parses <<<HERALD-REPLY>>>, and routes the action
// (reply | issue.open | event.emit).
//
// §107 posture (anti-bluff): the `listen` command's PASS is NOT "the
// process boots cleanly + exits cleanly" — that's a §11.4 PASS-bluff. The
// listen_test integration test asserts the handler is wired (≥1
// dispatcher.Handle invocation arrives via the stub subscriber within 8s)
// AND that SIGTERM / context cancel actually returns runListen cleanly.
// The runListen helper is the seam: production RunE constructs real
// dependencies and calls runListen; the test constructs stub dependencies
// and calls runListen directly (no binary spawn — fast + deterministic).
//
// Env-only configuration (no flags wired Wave 6; flag-or-env can land in
// HRD-NNN-W6c when an operator requests it):
//
//	HERALD_TGRAM_BOT_TOKEN — required: Telegram bot token (validated by
//	                         tgram.Subscribe via telebot.NewBot getMe).
//	HERALD_TGRAM_CHAT_ID   — required: numeric chat ID this binary serves
//	                         (tgram:// URL component; Adapter routing
//	                         ignores it for inbound, but the URL parser
//	                         requires it).
//	HERALD_PROJECT_NAME    — optional: pinned via commons.ProjectName().
//	HERALD_CLAUDE_BIN      — optional: path to `claude` binary (default
//	                         "claude" via $PATH per claude_code.New).
//	HERALD_INBOUND_CC_FAKE — listen_test seam: if "1", the runListen
//	                         helper substitutes a no-op CodeDispatcher
//	                         that returns a canned reply, so the test
//	                         never spawns the real claude CLI. Production
//	                         callers do NOT set this.
//	HERALD_OPERATOR_IDS    — Wave 6.5 T7: comma-separated channel_user_id
//	                         allowlist for the §32.6 Done:/Reopen:
//	                         commands. Empty / unset → both commands are
//	                         rejected with ErrNotOperator. Whitespace
//	                         tolerant; empty segments skipped. Stub for
//	                         V3 §32.10 subscriber-role mapping (Wave 7).
//
// Flags:
//
//	--docs-dir <path>      — Wave 6.5 T7: docs root containing Issues.md,
//	                         Fixed.md, Status.md, CONTINUATION.md, Help.md.
//	                         Default "docs" (relative to cwd). docs/Issues.md
//	                         MUST exist; the command refuses to boot
//	                         otherwise (§107 fail-loud — silent degradation
//	                         to "everything goes to CC" would mask a real
//	                         misconfiguration).
//	--qa-out-dir <path>    — Wave 6 T10a: JSONL journal directory.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/commons"
	"github.com/vasic-digital/herald/commons_messaging/channels"
	// tgram + slack are blank-imported for their init() registration with
	// the channels registry (Wave 7 T2 / T6) — pherald listen resolves
	// "tgram" and "slack" by name via channels.New, driven by HERALD_CHANNELS.
	_ "github.com/vasic-digital/herald/commons_messaging/channels/slack"
	_ "github.com/vasic-digital/herald/commons_messaging/channels/tgram"
	"github.com/vasic-digital/herald/commons_messaging/dispatch/claude_code"
	workable "github.com/vasic-digital/herald/commons_workable"
	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

// newListenCmd wires the `pherald listen` Cobra subcommand. Wave 6 keeps
// the surface env-only to match the operator-locked config story (single
// .env file controls everything); a follow-up HRD can lift values to
// --bot-token / --chat-id / --project-name flags if the workflow demands.
//
// Wave 6 T10a (2026-05-22) adds ONE flag — `--qa-out-dir <path>` — which,
// when set, journals every bidirectional inbound/CC/outbound event into
// `<path>/transcript.jsonl` per the docs/qa/ §107.x evidence mandate.
// Attachments are copied (content-addressed) under `<path>/attachments/`.
func newListenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Run the inbound runtime: Telegram getUpdates long-poll + Claude Code dispatch loop",
		Long: `Long-running. Wires tgram.Subscribe (getUpdates long-poll) to the
production inbound.Dispatcher (Wave 6 T7). Every inbound message is
dispatched through Claude Code (Opus, pinned) per the §32 inbound pipeline.

Required environment:
  HERALD_TGRAM_BOT_TOKEN   Telegram bot token (validated via getMe on boot).
  HERALD_TGRAM_CHAT_ID     Numeric chat ID; included in the tgram:// URL.

Optional environment:
  HERALD_PROJECT_NAME      Claude Code session name (else basename(cwd) else "Herald").
  HERALD_CLAUDE_BIN        Path to ` + "`claude`" + ` CLI (default: $PATH lookup).

Optional flags:
  --qa-out-dir <path>      Journal every inbound/CC/outbound event to
                           <path>/transcript.jsonl and copy attachments to
                           <path>/attachments/<sha256>.<ext>. Wave 6 T10a
                           §107.x evidence primitive — feature ships with
                           the resulting run dir under docs/qa/<run-id>/.
  --docs-dir <path>        Docs root containing Issues.md / Fixed.md /
                           Status.md / CONTINUATION.md / Help.md. Default
                           "docs" (relative to cwd). Issues.md MUST exist;
                           startup fails otherwise (Wave 6.5 T7 §107
                           fail-loud).

Signal handling: SIGINT/SIGTERM cancels the long-poll cleanly via
signal.NotifyContext.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// --channels (Wave 7, HRD-114b): when non-empty, OVERRIDES
			// HERALD_CHANNELS for this invocation; when empty, the env var
			// (default ["tgram"]) is used. This makes the documented
			// `pherald listen --channels slack` form work end-to-end.
			channelsFlag, _ := cmd.Flags().GetString("channels")
			cfg, err := loadListenConfigFromEnv(channelsFlag)
			if err != nil {
				return err
			}
			cfg.QAOutDir, _ = cmd.Flags().GetString("qa-out-dir")
			cfg.DocsDir, _ = cmd.Flags().GetString("docs-dir")
			if cfg.DocsDir == "" {
				cfg.DocsDir = "docs"
			}
			cfg.OperatorIDs = parseOperatorIDs(os.Getenv("HERALD_OPERATOR_IDS"))
			if err := wireDocsAndCommands(&cfg); err != nil {
				return err
			}
			// HRD-152 production wiring: open the workable-items SQLite SSoT
			// (mirrors `pherald watch`'s --db pattern) and back the
			// item.update / item.delete / confirmed-investigation actions with
			// a real RepoMutator. Empty/unset path → Items stays nil → those
			// actions return the dispatcher's explicit "no ItemMutator
			// configured" error (graceful preserved behaviour).
			dbPath, _ := cmd.Flags().GetString("db")
			closeDB, err := wireItemMutator(&cfg, dbPath)
			if err != nil {
				return err
			}
			if closeDB != nil {
				defer closeDB()
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			enabledNames := make([]string, 0, len(cfg.Subscribers))
			for name := range cfg.Subscribers {
				enabledNames = append(enabledNames, name)
			}
			sort.Strings(enabledNames)
			fmt.Fprintf(cmd.OutOrStdout(), "pherald listen: starting inbound runtime for channel(s): %s\n", strings.Join(enabledNames, ", "))
			if cfg.QAOutDir != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "pherald listen: QA journaling enabled — %s\n", cfg.QAOutDir)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pherald listen: docs dir %s; %d operator(s) authorised for Done:/Reopen:\n",
				cfg.DocsDir, len(cfg.OperatorIDs))
			if cfg.Items != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "pherald listen: workable-item mutation enabled (item.update/item.delete/confirmed-investigation)")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "pherald listen: workable-item mutation DISABLED (no DB) — item.update/item.delete/investigation actions will error")
			}
			return runListen(ctx, cfg)
		},
	}
	cmd.Flags().String("channels", "",
		"Comma-separated channel set to bring up (e.g. \"slack\" or \"tgram,slack\"). Overrides HERALD_CHANNELS when set; empty falls back to the env var (default tgram). Wave 7 HRD-114b.")
	cmd.Flags().String("qa-out-dir", "",
		"If set, journal inbound/CC/outbound events to <dir>/transcript.jsonl + copy attachments to <dir>/attachments/<sha256>.<ext> (Wave 6 T10a §107.x evidence)")
	cmd.Flags().String("docs-dir", "docs",
		"Docs root containing Issues.md / Fixed.md / Status.md / CONTINUATION.md / Help.md. Issues.md MUST exist (Wave 6.5 T7).")
	cmd.Flags().String("db", "",
		"Workable-items SQLite DB path backing item.update/item.delete/confirmed-investigation actions (default $HERALD_WORKABLE_DB or docs/workable_items.db; HRD-152). Empty path with no env → those actions return an explicit \"no ItemMutator configured\" error.")
	return cmd
}

// wireItemMutator resolves the workable-items DB path (HRD-152 production
// wiring) and, when it resolves to a non-empty path, opens the SQLite SSoT
// and stamps a real inbound.RepoMutator into cfg.Items — mirroring
// `pherald watch`'s --db open pattern (commons_workable.Open + NewRepo).
//
// Resolution order (matches watch.go): the explicit dbPath flag value first;
// then $HERALD_WORKABLE_DB; then the canonical default "docs/workable_items.db".
// A flag/env value of "" with the env unset still resolves to the default,
// so the production path always wires Items unless the operator explicitly
// passes --db "" AND unsets the env — in which case Items stays nil and the
// item.update/item.delete/investigation actions return the dispatcher's
// explicit "no ItemMutator configured" error (graceful preserved behaviour;
// never a silent drop, §107 fail-loud).
//
// Returns a close func (the caller defers it to release the DB handle on
// shutdown) or nil when no DB was opened.
func wireItemMutator(cfg *listenConfig, dbPath string) (func(), error) {
	if dbPath == "" {
		if env := os.Getenv("HERALD_WORKABLE_DB"); env != "" {
			dbPath = env
		} else {
			dbPath = "docs/workable_items.db"
		}
	}
	if dbPath == "" {
		// Operator explicitly disabled the DB (passed --db "" with the env
		// also empty would already have defaulted above; this guards a future
		// caller that sets dbPath="" intentionally). Leave Items nil.
		return nil, nil
	}
	store, err := workable.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("pherald listen: open workable DB %q: %w", dbPath, err)
	}
	cfg.Items = inbound.NewRepoMutator(workable.NewRepo(store))
	return func() { _ = store.Close() }, nil
}

// listenConfig carries the resolved dependencies for runListen. The
// Subscriber + Code fields are the test seams: production code populates
// them with the live tgram.Adapter.Subscribe + claude_code.Dispatcher
// (wrapped by inbound.NewCCAdapter); listen_test populates them with
// stubs so the dispatch loop can be driven hermetically.
type listenConfig struct {
	ProjectName string
	BotToken    string
	ChatID      string
	ClaudeBin   string
	// QAOutDir, when non-empty, enables JSONL journaling of every
	// bidirectional event to <QAOutDir>/transcript.jsonl plus content-
	// addressed attachment copies under <QAOutDir>/attachments/ per the
	// Wave 6 T10a §107.x evidence primitive.
	QAOutDir string
	// DocsDir is the operator-supplied docs root (Wave 6.5 T7). Default
	// "docs"; --docs-dir overrides. Issues.md must exist inside it.
	DocsDir string
	// OperatorIDs is the parsed HERALD_OPERATOR_IDS env (Wave 6.5 T7).
	// Empty / nil → all Done:/Reopen: commands rejected.
	OperatorIDs map[string]bool
	// IssueOpener (Wave 6.5 T7) wires action=issue.open through to
	// docs/Issues.md. Constructed in wireDocsAndCommands when the
	// docs dir is valid; nil in hermetic tests that don't exercise
	// the issue.open action.
	IssueOpener inbound.IssueOpener
	// Commands (Wave 6.5 T7) wires the §32.6 fast-path command handlers.
	// Constructed in wireDocsAndCommands; nil in hermetic tests that
	// don't exercise the command fast-path.
	Commands *inbound.CommandsConfig
	// EventEmitter (Wave 6.5 T7) wires action=event.emit through to
	// runner.Runner.Run. Wave 6.5 ships this nil — the operator runs
	// pherald listen + pherald serve in separate processes today, so
	// the listen-side Runner is unavailable. The dispatcher returns
	// an explicit "no EventEmitter configured" error for event.emit
	// replies when this is nil (§107 fail-loud).
	EventEmitter inbound.EventEmitter
	// Subscribers maps each enabled channel name → its long-poll entry point
	// (Wave 7 HRD-114). runListen launches one goroutine per entry and
	// fans them ALL into the SAME inbound.Dispatcher. Production:
	// Subscribers["tgram"] = tgramAdapter.Subscribe (and "slack" etc. once
	// the adapter lands). Test: stub closures that publish one synthetic
	// InboundEvent and wait for ctx.Done. Default (HERALD_CHANNELS unset) is
	// a single "tgram" entry, preserving Wave 6 single-channel behaviour.
	Subscribers map[string]func(ctx context.Context, h commons.InboundHandler) error
	// Code is the CC dispatcher. Production: inbound.NewCCAdapter(*claude_code.Dispatcher).
	// Test: a stubCode returning a canned <<<HERALD-REPLY>>> blob.
	Code inbound.CodeDispatcher
	// Replier is the outbound reply binding (Wave 7 generic inbound.Replier).
	// Production: a channelRouter that dispatches each reply to the adapter
	// for the recipient's channel (so a tgram event replies via tgram, a
	// slack event via slack). Test: a recordingReplier.
	Replier inbound.Replier
	// Items is the workable-item CRUD boundary (WS-4 / HRD-152, production
	// wiring HRD-152). Non-nil when --db / HERALD_WORKABLE_DB resolves to a
	// path: it backs the item.update / item.delete / confirmed-investigation
	// mutation actions, mirroring `pherald watch`'s DB-open pattern. When nil
	// (DB path empty/unset), those actions return the dispatcher's explicit
	// "no ItemMutator configured" error — graceful preserved behaviour, never
	// a silent drop (§107 fail-loud). Hermetic listen_test leaves it nil.
	Items inbound.ItemMutator
	// Resolver is the §11.4.104 participant identity resolver (GAP G1/G2).
	// Production wires an env-backed commons.MemoryResolver (operator handle
	// per channel via OperatorHandleFromEnv) so created_by/assigned_to
	// attribution (§2) and the §110 Tier-3 clarify-tag @username resolution
	// run for EVERY channel (Slack AND Telegram). When nil, the dispatcher
	// skips attribution + falls back to the raw sender handle for clarify
	// tags (Wave 6 behaviour preserved). Hermetic tests may inject a stub or
	// leave it nil.
	Resolver commons.IdentityResolver
}

// loadListenConfigFromEnv resolves env into listenConfig + constructs the
// production tgram + claude_code dependencies. Returns a fully wired
// listenConfig on success. Returns a descriptive error if any required
// env var is missing — never a "boots silently with anonymous defaults"
// path (anonymous serve plane is a §107 PASS-bluff per main.go's
// buildVerifier doc-comment).
//
// channelsOverride (Wave 7 HRD-114b) is the --channels flag value: when
// non-empty it OVERRIDES HERALD_CHANNELS; when empty the env var is read
// (default ["tgram"]). Production passes cmd.Flags().GetString("channels");
// tests pass "" to exercise the env path or an explicit string to exercise
// the override path.
func loadListenConfigFromEnv(channelsOverride string) (listenConfig, error) {
	projectName := commons.ProjectName()
	claudeBin := os.Getenv("HERALD_CLAUDE_BIN")

	// Wave 7 (HRD-114): resolve the enabled channel set from the --channels
	// flag (override) or HERALD_CHANNELS (comma-split; default ["tgram"] when
	// both are unset/empty). Each enabled channel is constructed via the
	// channels.New registry (Wave 7 T2) with its namespaced env
	// (perChannelConfig). The Subscribers map fans them all into one
	// Dispatcher; a channelRouter routes each reply back to the adapter for
	// the recipient's channel.
	enabled := resolveChannelListWithOverride(channelsOverride)
	subscribers := map[string]func(ctx context.Context, h commons.InboundHandler) error{}
	repliers := map[string]inbound.Replier{}

	// BotToken / ChatID are surfaced on the config for observability + the
	// existing single-channel fields; we capture the tgram values when tgram
	// is enabled so the doctor/version surfaces keep working.
	var botToken, chatID string

	for _, name := range enabled {
		ccfg, err := perChannelConfig(name)
		if err != nil {
			return listenConfig{}, err
		}
		ch, err := channels.New(name, ccfg)
		if err != nil {
			return listenConfig{}, fmt.Errorf("pherald listen: build %q channel: %w", name, err)
		}
		subscribers[name] = ch.Subscribe
		// Each channels.Channel exposes SendReplyGeneric (the generic reply
		// shape). Wrap it as an inbound.Replier so the dispatcher (which
		// names the method SendReply) can call it without knowing the
		// concrete channel type.
		repliers[name] = &channelReplier{ch: ch}
		if name == string(commons.ChannelTelegram) {
			botToken = ccfg.Token
			chatID = ccfg.Target
		}
	}
	if len(subscribers) == 0 {
		// loadEnabledChannels guarantees ≥1, but fail loud if a future
		// refactor breaks that invariant (silent no-channel boot is a §107
		// PASS-bluff — the process would idle forever, appearing healthy).
		return listenConfig{}, fmt.Errorf("pherald listen: no channels enabled (HERALD_CHANNELS resolved empty)")
	}

	// Build the production Claude Code dispatcher.
	ccDispatcher, err := claude_code.New(claudeBin, "", projectName)
	if err != nil {
		return listenConfig{}, fmt.Errorf("pherald listen: build claude_code dispatcher: %w", err)
	}

	cfg := listenConfig{
		ProjectName: projectName,
		BotToken:    botToken,
		ChatID:      chatID,
		ClaudeBin:   claudeBin,
		Subscribers: subscribers,
		Code:        inbound.NewCCAdapter(ccDispatcher),
		Replier:     &channelRouter{repliers: repliers},
		// GAP G1/G2: build the §11.4.104 participant resolver from the
		// enabled channel set so attribution + clarify-tagging run for every
		// channel (Slack AND Telegram), consuming HERALD_<CHANNEL>_OPERATOR_USERNAME.
		Resolver: buildResolver(enabled),
	}

	// HERALD_INBOUND_CC_FAKE is the listen_test seam — see package doc.
	// Production callers do NOT set it; the env-read here is the entire
	// kill-switch surface.
	if os.Getenv("HERALD_INBOUND_CC_FAKE") == "1" {
		cfg.Code = fakeCodeDispatcher{}
	}

	return cfg, nil
}

// loadEnabledChannels reads HERALD_CHANNELS (comma-separated channel names)
// and returns the trimmed, non-empty, de-duplicated set in declaration order.
// Unset / empty → ["tgram"] (Wave 6 single-channel behaviour preserved).
// Whitespace around each name is trimmed; empty segments
// (leading/trailing/double commas) are skipped.
func loadEnabledChannels() []string {
	return resolveChannelList(os.Getenv("HERALD_CHANNELS"))
}

// resolveChannelListWithOverride implements the Wave 7 HRD-114b source
// precedence for `pherald listen`: the override (the --channels flag value)
// wins when non-empty; otherwise HERALD_CHANNELS is read. When both are
// empty/unset the default ["tgram"] is returned. Kept separate from
// loadEnabledChannels so other callers (pherald watch) that have no
// --channels flag keep the env-only behaviour unchanged.
func resolveChannelListWithOverride(override string) []string {
	if strings.TrimSpace(override) != "" {
		return resolveChannelList(override)
	}
	return resolveChannelList(os.Getenv("HERALD_CHANNELS"))
}

// resolveChannelList parses a comma-separated channel-name string into the
// trimmed, non-empty, de-duplicated set in declaration order, defaulting to
// ["tgram"] when nothing remains.
func resolveChannelList(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{string(commons.ChannelTelegram)}
	}
	return out
}

// perChannelConfig reads the namespaced env for the named channel and
// returns the channels.Config the registry constructor consumes. Required
// credentials missing → explicit error (§107 fail-loud: a channel that
// boots with empty creds is a PASS-bluff — getUpdates/auth.test would fail
// silently at first poll). Unknown channel names are NOT rejected here —
// channels.New surfaces ErrUnknownChannel with the registered set.
func perChannelConfig(name string) (channels.Config, error) {
	switch name {
	case string(commons.ChannelTelegram):
		token := os.Getenv("HERALD_TGRAM_BOT_TOKEN")
		if token == "" {
			return channels.Config{}, fmt.Errorf("pherald listen: HERALD_TGRAM_BOT_TOKEN required (channel tgram enabled)")
		}
		chatID := os.Getenv("HERALD_TGRAM_CHAT_ID")
		if chatID == "" {
			return channels.Config{}, fmt.Errorf("pherald listen: HERALD_TGRAM_CHAT_ID required (channel tgram enabled)")
		}
		return channels.Config{Token: token, Target: chatID}, nil
	case "slack":
		token := os.Getenv("HERALD_SLACK_BOT_TOKEN")
		if token == "" {
			return channels.Config{}, fmt.Errorf("pherald listen: HERALD_SLACK_BOT_TOKEN required (channel slack enabled)")
		}
		return channels.Config{
			Token:    token,
			AppToken: os.Getenv("HERALD_SLACK_APP_TOKEN"),
			Target:   os.Getenv("HERALD_SLACK_CHANNEL_ID"),
		}, nil
	default:
		// Generic fallback: pass the single-token namespaced env if present.
		// channels.New rejects truly-unknown names with ErrUnknownChannel.
		return channels.Config{
			Token:  os.Getenv("HERALD_" + strings.ToUpper(name) + "_BOT_TOKEN"),
			Target: os.Getenv("HERALD_" + strings.ToUpper(name) + "_CHANNEL_ID"),
		}, nil
	}
}

// channelReplier adapts a channels.Channel (whose reply method is named
// SendReplyGeneric to keep *tgram.Adapter satisfying channels.Channel
// without shadowing its native int64 SendReply — see channels/channel.go's
// package doc) to the inbound.Replier interface (whose method is named
// SendReply). This thin shim is the bridge between the two intentionally-
// differently-named generic methods.
type channelReplier struct{ ch channels.Channel }

func (r *channelReplier) SendReply(ctx context.Context, recipient commons.Recipient, body, replyToID string, atts []commons.Attachment) (string, error) {
	return r.ch.SendReplyGeneric(ctx, recipient, body, replyToID, atts)
}

// channelRouter implements inbound.Replier by routing each reply to the
// adapter registered for recipient.Channel. A reply for an unregistered
// channel is a §107 fail-loud error (never silently dropped — a dropped
// reply is the canonical "everything went green but the user got nothing"
// bluff).
type channelRouter struct{ repliers map[string]inbound.Replier }

func (r *channelRouter) SendReply(ctx context.Context, recipient commons.Recipient, body, replyToID string, atts []commons.Attachment) (string, error) {
	rep, ok := r.repliers[recipient.Channel]
	if !ok {
		return "", fmt.Errorf("pherald listen: no replier for channel %q (registered: %v)", recipient.Channel, channelRouterKeys(r.repliers))
	}
	return rep.SendReply(ctx, recipient, body, replyToID, atts)
}

// channelRouterKeys returns the routed channel names for the fail-loud
// error message above (sorted for deterministic test output).
func channelRouterKeys(m map[string]inbound.Replier) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// runListen wires the inbound.Dispatcher to every configured Subscriber and
// blocks until ctx is cancelled (or a subscriber dies). Extracted from RunE
// so listen_test can drive it hermetically without binary spawn.
//
// Wave 7 (HRD-114): fan-in. cfg.Subscribers maps each enabled channel name →
// its long-poll entry point; runListen launches one goroutine per entry, all
// feeding the SAME dispatcher (one CC session, one reply router). If ANY
// subscriber returns a non-cancel error, runListen cancels the siblings and
// surfaces that error (T11 chaos fail-loud — a channel that silently dies
// must take the process down, not idle the other channels while one is dark).
//
// §107 anchor: the loop is NOT "boots + exits" — runListen returns the
// error from the subscribers (each is itself the long-poll loop that invokes
// Handle on every message). The multi-channel test asserts BOTH channels'
// synthetic events reach the dispatcher AND both replies are recorded — not
// merely "both goroutines started".
func runListen(ctx context.Context, cfg listenConfig) error {
	code := cfg.Code
	replier := cfg.Replier
	var jrn *journal
	if cfg.QAOutDir != "" {
		var jerr error
		jrn, jerr = newJournal(cfg.QAOutDir)
		if jerr != nil {
			return fmt.Errorf("pherald listen: open journal: %w", jerr)
		}
		defer jrn.Close()
		code = &journalingCode{j: jrn, inner: code}
		replier = &journalingReplier{j: jrn, inner: replier}
	}
	dispatcher, err := inbound.NewDispatcher(inbound.Config{
		ProjectName: cfg.ProjectName,
		Code:        code,
		Reply:       replier,
		Issues:      cfg.IssueOpener,
		Events:      cfg.EventEmitter,
		Commands:    cfg.Commands,
		Items:       cfg.Items,
		// GAP G1/G2: thread the participant resolver so item.update injects
		// created_by/assigned_to (§2) and the §110 Tier-3 clarify reply tags
		// the sender's @username. nil is tolerated by the dispatcher (Wave 6
		// behaviour) but production always wires a non-nil resolver here.
		Resolver: cfg.Resolver,
	})
	if err != nil {
		return fmt.Errorf("pherald listen: build inbound dispatcher: %w", err)
	}
	var handler commons.InboundHandler = dispatcher
	if jrn != nil {
		handler = &journalingHandler{j: jrn, inner: handler}
	}

	if len(cfg.Subscribers) == 0 {
		return fmt.Errorf("pherald listen: no subscribers configured")
	}

	// Fan-in: one goroutine per channel subscriber, all sharing handler.
	// A child context lets the first failing subscriber cancel its siblings.
	gctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, len(cfg.Subscribers))
	for name, sub := range cfg.Subscribers {
		name, sub := name, sub
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sub(gctx, handler); err != nil {
				// A subscriber returns ctx.Err() on clean cancellation —
				// that is NOT a failure. Any other error is a channel death:
				// cancel the siblings + surface the error (fail-loud).
				if gctx.Err() != nil {
					return
				}
				cancel()
				errCh <- fmt.Errorf("pherald listen: channel %q subscribe: %w", name, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}
	return nil
}

// fakeCodeDispatcher is the HERALD_INBOUND_CC_FAKE=1 seam. Returns a
// canned <<<HERALD-REPLY>>> blob so the inbound.Dispatcher routes to the
// "reply" action without spawning a real claude binary. Production code
// paths never instantiate this type — the only construction site is
// loadListenConfigFromEnv when the env opt-in is set.
type fakeCodeDispatcher struct{}

func (fakeCodeDispatcher) Dispatch(_ context.Context, _ inbound.CodeRequest) (inbound.CodeResponse, error) {
	return inbound.CodeResponse{
		Stdout: []byte(`<<<HERALD-REPLY>>> {"action":"reply","text":"ack (fake)"}`),
	}, nil
}

// parseOperatorIDs is the §32.6 Done:/Reopen: operator-role allowlist
// parser (Wave 6.5 T7). It accepts a comma-separated string of
// channel_user_id values (Telegram numeric chat IDs) and returns the
// set as a map[string]bool. Whitespace around each segment is trimmed;
// empty segments (including ",,", trailing/leading commas) are skipped.
//
// Empty / unset HERALD_OPERATOR_IDS env → empty map → all Done:/Reopen:
// commands rejected with inbound.ErrNotOperator. The pherald listen
// startup line prints the count so the operator sees the count before
// any inbound message arrives.
func parseOperatorIDs(s string) map[string]bool {
	m := map[string]bool{}
	if s == "" {
		return m
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m[p] = true
	}
	return m
}

// wireDocsAndCommands constructs the Wave 6.5 T7 docs-aware dependencies
// (DocsIssueOpener + CommandsConfig) from cfg.DocsDir + cfg.OperatorIDs
// and stamps them into cfg in-place. Returns an explicit error if
// docs/Issues.md is missing — §107 fail-loud, never silent-degrade to
// "everything goes to CC because the issue opener can't see the file".
//
// Optional siblings (Status.md / CONTINUATION.md / Help.md) emit
// warnings to stderr but do not block startup; the corresponding
// commands fall back to errors-at-call-time (HandleStatus / HandleContinue
// return an error from os.ReadFile when invoked; HandleHelp falls back
// to BuiltinHelp by design).
//
// EventEmitter is left nil — the operator runs pherald listen +
// pherald serve in separate processes today (Wave 6.5 scope). The
// dispatcher returns an explicit error for action=event.emit replies
// when nil, which the operator sees as a SendReply error.
func wireDocsAndCommands(cfg *listenConfig) error {
	if cfg.DocsDir == "" {
		cfg.DocsDir = "docs"
	}
	issuesPath := filepath.Join(cfg.DocsDir, "Issues.md")
	fixedPath := filepath.Join(cfg.DocsDir, "Fixed.md")
	if _, err := os.Stat(issuesPath); err != nil {
		return fmt.Errorf("pherald listen: docs Issues.md not found at %s — set --docs-dir or chdir to repo root: %w",
			issuesPath, err)
	}
	if _, err := os.Stat(fixedPath); err != nil {
		// Fixed.md absent is a warning, not fatal — nextHRDNumber
		// tolerates the missing path. But Done:/Reopen: will fail
		// when invoked against a missing Fixed.md (with explicit
		// error to the operator), so warn at startup so the
		// operator notices.
		fmt.Fprintf(os.Stderr, "pherald listen: warning — Fixed.md missing at %s (Done:/Reopen: will fail until present)\n", fixedPath)
	}
	statusPath := filepath.Join(cfg.DocsDir, "Status.md")
	continuePath := filepath.Join(cfg.DocsDir, "CONTINUATION.md")
	helpPath := filepath.Join(cfg.DocsDir, "Help.md")
	if _, err := os.Stat(statusPath); err != nil {
		fmt.Fprintf(os.Stderr, "pherald listen: warning — Status.md missing at %s (Status: command will error until present)\n", statusPath)
	}
	if _, err := os.Stat(continuePath); err != nil {
		fmt.Fprintf(os.Stderr, "pherald listen: warning — CONTINUATION.md missing at %s (Continue: command will error until present)\n", continuePath)
	}
	// Help.md is optional by design (BuiltinHelp fallback) — no warning.
	lockPath := filepath.Join(cfg.DocsDir, ".issues.lock")
	cfg.IssueOpener = &inbound.DocsIssueOpener{
		IssuesPath: issuesPath,
		FixedPath:  fixedPath,
		LockPath:   lockPath,
		Clock:      commons.RealClock{},
	}
	cfg.Commands = &inbound.CommandsConfig{
		DocsDir:      cfg.DocsDir,
		IssuesPath:   issuesPath,
		FixedPath:    fixedPath,
		StatusPath:   statusPath,
		ContinuePath: continuePath,
		HelpPath:     helpPath,
		LockPath:     lockPath,
		OperatorIDs:  cfg.OperatorIDs,
		Clock:        commons.RealClock{},
	}
	cfg.EventEmitter = nil
	return nil
}

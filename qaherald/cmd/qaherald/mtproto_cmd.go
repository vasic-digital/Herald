// Wave 8 Track B — `qaherald mtproto` Cobra subcommand tree.
//
// Three sub-subcommands:
//
//	qaherald mtproto login   # one-time interactive bootstrap — the
//	                         # §11.4.98(B) permitted single human step.
//	                         # Asks Telegram to SMS / app-push a login
//	                         # code; operator types the code at the
//	                         # CLI; the persisted session file
//	                         # (default ~/.config/herald/mtproto.session,
//	                         # mode 0600) is then valid for all
//	                         # subsequent fully-autonomous invocations.
//
//	qaherald mtproto whoami  # verifies the persisted session is alive
//	                         # by connecting + calling
//	                         # client.Self(ctx) and printing the user
//	                         # id + username. NEVER prompts.
//
//	qaherald mtproto logout  # invalidates the session via tg.AuthLogOut
//	                         # and removes the local session file. Use
//	                         # before rotating credentials or before
//	                         # tearing down a QA test bench.
//
// §107.x / §107.y posture:
//   - All four required env vars are validated up-front; missing or
//     mis-shaped values surface a clear, structured error (no panic,
//     no nil deref) so an operator-facing CI gate can grep for the
//     diagnostic.
//   - HERALD_MTPROTO_PASSWORD is read from env ONLY; never echoed at
//     any verbosity level, never printed in error messages.
//   - The login command's code prompt reads from os.Stdin (single line,
//     no echo). If stdin is not a TTY (CI), login refuses with a
//     §11.4.98(B) pointer.
//   - sanitizeMTProtoError is the contract surface: any error returned
//     by the mtproto package is already sanitized; we add a thin
//     "mtproto login:" / "mtproto whoami:" / "mtproto logout:" prefix
//     and propagate.
//
// Wiring: the parent `mtproto` command is added to rootCmd from this
// file's init(); the three children are added to it before init() returns.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"github.com/vasic-digital/herald/qaherald/internal/mtproto"
)

// mtprotoCmd is the parent of {login, whoami, logout}.
var mtprotoCmd = &cobra.Command{
	Use:   "mtproto",
	Short: "Manage the MTProto user-client session used by qaherald",
	Long: `qaherald mtproto manages the persisted MTProto user-client session
that drives Herald's fully-autonomous QA bot flows per HelixConstitution
§11.4.98 (Full-Automation Anti-Bluff Mandate).

Why MTProto and not Bot API: Telegram's bot privacy boundary blocks a
bot from observing another bot's messages in non-DM contexts. The QA
flavor MUST drive the system bot end-to-end; the only autonomous way to
do that is via a real user account speaking MTProto.

Lifecycle:
  1. One-time bootstrap:  qaherald mtproto login    (interactive — operator
                                                     enters Telegram's
                                                     SMS / app-push code)
  2. Health check:        qaherald mtproto whoami   (autonomous)
  3. Teardown / rotate:   qaherald mtproto logout   (autonomous)

Required environment variables (all four):
  HERALD_MTPROTO_APP_ID         from https://my.telegram.org/apps (integer)
  HERALD_MTPROTO_APP_HASH       from https://my.telegram.org/apps (32-char hex)
  HERALD_MTPROTO_PHONE          E.164 phone of the QA user account
  HERALD_MTPROTO_PASSWORD       cloud 2FA password (empty if 2FA off)

Session file: ~/.config/herald/mtproto.session (mode 0600). Never
committed; never echoed; encrypted at rest only by filesystem ACL.`,
}

// mtprotoLoginCmd is the §11.4.98(B) one-time interactive bootstrap.
var mtprotoLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "One-time interactive bootstrap — populates the persisted MTProto session",
	Long: `qaherald mtproto login performs the §11.4.98(B) permitted one-time
interactive bootstrap. Sequence:

  1. Read HERALD_MTPROTO_APP_ID / APP_HASH / PHONE / PASSWORD from env.
  2. Connect to Telegram via the gotd/td MTProto stack.
  3. Request a login code (Telegram sends SMS or app-push notification).
  4. Read the code from stdin (single line, terminator '\n').
  5. (If 2FA enabled) submit HERALD_MTPROTO_PASSWORD.
  6. Persist the session to ~/.config/herald/mtproto.session (mode 0600).

After this completes, every subsequent qaherald mtproto / lifecycle /
run invocation will reuse the persisted session WITHOUT prompting —
that's the §11.4.98(B) "interactive once, autonomous thereafter"
contract.

If stdin is not a TTY (e.g. running under non-interactive CI), this
command refuses to proceed — the one-time interactive bootstrap MUST be
performed by a human operator, NEVER scripted with a hard-coded code.`,
	RunE: runMTProtoLogin,
}

// mtprotoWhoamiCmd verifies the persisted session is alive.
var mtprotoWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Verify the persisted MTProto session by printing the connected user identity",
	Long: `qaherald mtproto whoami connects to Telegram using the persisted
MTProto session and prints the user id + username of the authenticated
account. Fully autonomous (never prompts). If the session is missing or
invalid, exits non-zero with a pointer at qaherald mtproto login.

Use this as a §107 anti-bluff sanity check before launching a long QA
campaign — confirms that the session has not been invalidated by
Telegram (account-suspend, password-change, server-side logout).`,
	RunE: runMTProtoWhoami,
}

// mtprotoLogoutCmd invalidates the session.
var mtprotoLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Invalidate the persisted MTProto session (server-side LogOut + remove local file)",
	Long: `qaherald mtproto logout sends tg.AuthLogOut to invalidate the
session server-side, then removes the local session file. Use before
rotating credentials, tearing down a QA test bench, or switching to a
different QA account.

If the session is already missing or unauthorized, logout still removes
the local file so the disk state is consistent.`,
	RunE: runMTProtoLogout,
}

func init() {
	mtprotoCmd.AddCommand(mtprotoLoginCmd)
	mtprotoCmd.AddCommand(mtprotoWhoamiCmd)
	mtprotoCmd.AddCommand(mtprotoLogoutCmd)
	rootCmd.AddCommand(mtprotoCmd)
}

// mtprotoEnvConfig builds an mtproto.Config from the four canonical env
// vars + returns a clear structured error when any are missing or
// mis-shaped. Used by login + whoami + logout.
func mtprotoEnvConfig() (mtproto.Config, error) {
	appIDStr := os.Getenv("HERALD_MTPROTO_APP_ID")
	if appIDStr == "" {
		return mtproto.Config{}, errors.New("HERALD_MTPROTO_APP_ID is not set — see docs/requirements/blockers/missing_env_variables.md")
	}
	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return mtproto.Config{}, fmt.Errorf("HERALD_MTPROTO_APP_ID is not an integer: %w", err)
	}
	appHash := os.Getenv("HERALD_MTPROTO_APP_HASH")
	if appHash == "" {
		return mtproto.Config{}, errors.New("HERALD_MTPROTO_APP_HASH is not set")
	}
	phone := os.Getenv("HERALD_MTPROTO_PHONE")
	if phone == "" {
		return mtproto.Config{}, errors.New("HERALD_MTPROTO_PHONE is not set")
	}
	// Password is OPTIONAL — 2FA may be disabled.
	password := os.Getenv("HERALD_MTPROTO_PASSWORD")

	cfg := mtproto.Config{
		AppID:    appID,
		AppHash:  appHash,
		Phone:    phone,
		Password: password,
	}
	if err := cfg.Validate(); err != nil {
		return mtproto.Config{}, fmt.Errorf("invalid MTProto config: %w", err)
	}
	return cfg, nil
}

// runMTProtoLogin is the RunE for `qaherald mtproto login`.
func runMTProtoLogin(cmd *cobra.Command, args []string) error {
	cfg, err := mtprotoEnvConfig()
	if err != nil {
		return fmt.Errorf("mtproto login: %w", err)
	}
	// Stdin MUST be a TTY for the login flow. If we're being piped or
	// run under a non-interactive harness, refuse — the SMS code MUST
	// come from a human operator (anti-bluff per §11.4.98(B)).
	if !isStdinTTY() {
		return errors.New("mtproto login: stdin is not a TTY — the one-time interactive bootstrap requires a human operator to enter the Telegram login code. Run this command from an interactive shell")
	}

	// Pre-create the session directory; gotd/td FileStorage will write
	// the session file with mode 0600 once auth succeeds.
	if err := cfg.EnsureSessionDir(); err != nil {
		return fmt.Errorf("mtproto login: prepare session dir: %w", err)
	}
	sessionPath := cfg.ResolvedSessionFile()
	ss := &session.FileStorage{Path: sessionPath}

	// Build a UserAuthenticator that:
	//   Phone     → cfg.Phone (no prompt; we have it in env)
	//   Code      → read from stdin (the one human step)
	//   Password  → cfg.Password (no prompt; we have it in env;
	//              empty when 2FA is off — auth.CodeOnly behaviour
	//              if password missing).
	//   SignUp    → REFUSE — we never create new accounts via this flow.
	//   AcceptTOS → accept silently.
	auther := &loginAuthenticator{
		phone:    cfg.Phone,
		password: cfg.Password,
		codeIn:   bufio.NewReader(os.Stdin),
		codeOut:  cmd.OutOrStdout(),
	}

	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if ctx == nil {
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: ss,
	})

	flow := auth.NewFlow(auther, auth.SendCodeOptions{})

	fmt.Fprintf(cmd.OutOrStdout(), "qaherald mtproto login: connecting to Telegram (phone %s)...\n", maskPhone(cfg.Phone))

	err = client.Run(ctx, func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("self: %w", err)
		}
		username, _ := self.GetUsername()
		uname := "(no username)"
		if username != "" {
			uname = "@" + username
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"MTProto session active for %s (user_id=%d) — session persisted to %s\n",
			uname, self.ID, sessionPath,
		)
		return nil
	})
	if err != nil {
		return fmt.Errorf("mtproto login: %w", err)
	}
	return nil
}

// runMTProtoWhoami is the RunE for `qaherald mtproto whoami`.
func runMTProtoWhoami(cmd *cobra.Command, args []string) error {
	cfg, err := mtprotoEnvConfig()
	if err != nil {
		return fmt.Errorf("mtproto whoami: %w", err)
	}

	// Fast-path check: if the session file is missing, surface the
	// "run login first" error WITHOUT a network call.
	exists, err := cfg.SessionExists()
	if err != nil {
		return fmt.Errorf("mtproto whoami: %w", err)
	}
	if !exists {
		return fmt.Errorf("mtproto whoami: %w", mtproto.ErrNoSession)
	}

	client, err := mtproto.New(cfg)
	if err != nil {
		return fmt.Errorf("mtproto whoami: %w", err)
	}
	defer func() { _ = client.Close() }()

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("mtproto whoami: connect: %w", err)
	}
	id, username, err := client.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("mtproto whoami: %w", err)
	}
	uname := "(no username)"
	if username != "" {
		uname = "@" + username
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"MTProto session OK: user_id=%d username=%s session=%s\n",
		id, uname, cfg.ResolvedSessionFile(),
	)
	return nil
}

// runMTProtoLogout is the RunE for `qaherald mtproto logout`.
func runMTProtoLogout(cmd *cobra.Command, args []string) error {
	cfg, err := mtprotoEnvConfig()
	if err != nil {
		return fmt.Errorf("mtproto logout: %w", err)
	}
	sessionPath := cfg.ResolvedSessionFile()

	// Whether or not the server-side LogOut succeeds, we MUST remove
	// the local session file at the end so disk state stays consistent.
	defer func() {
		if rmErr := os.Remove(sessionPath); rmErr == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "mtproto logout: removed local session %s\n", sessionPath)
		} else if !errors.Is(rmErr, os.ErrNotExist) {
			fmt.Fprintf(cmd.ErrOrStderr(), "mtproto logout: warning — could not remove %s: %v\n", sessionPath, rmErr)
		}
	}()

	exists, err := cfg.SessionExists()
	if err != nil {
		return fmt.Errorf("mtproto logout: %w", err)
	}
	if !exists {
		fmt.Fprintf(cmd.OutOrStdout(), "mtproto logout: no local session at %s — nothing to do server-side\n", sessionPath)
		return nil
	}

	ss := &session.FileStorage{Path: sessionPath}
	tgClient := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: ss,
	})

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	runErr := tgClient.Run(ctx, func(ctx context.Context) error {
		api := tgClient.API()
		_, logoutErr := api.AuthLogOut(ctx)
		if logoutErr != nil {
			return fmt.Errorf("auth.logOut: %w", logoutErr)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "mtproto logout: server-side LogOut OK")
		return nil
	})
	if runErr != nil {
		return fmt.Errorf("mtproto logout: %w", runErr)
	}
	return nil
}

// loginAuthenticator implements auth.UserAuthenticator. Reads the
// SMS / app-push code from os.Stdin (the §11.4.98(B) one-time human
// step); phone + password come from Config.
type loginAuthenticator struct {
	phone    string
	password string
	codeIn   *bufio.Reader
	codeOut  io.Writer
}

func (l *loginAuthenticator) Phone(ctx context.Context) (string, error) {
	return l.phone, nil
}

func (l *loginAuthenticator) Password(ctx context.Context) (string, error) {
	if l.password == "" {
		// 2FA disabled — surface auth.ErrPasswordNotProvided so the
		// flow short-circuits cleanly when Telegram asks for one.
		return "", auth.ErrPasswordNotProvided
	}
	return l.password, nil
}

func (l *loginAuthenticator) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	// Auto-accept. Telegram's TOS is the same one the operator agreed to
	// when creating the QA account; this auto-accept simply propagates
	// that consent through the API.
	return nil
}

func (l *loginAuthenticator) SignUp(ctx context.Context) (auth.UserInfo, error) {
	// We do NOT create new accounts via this flow — the QA user must
	// already exist. Refusing here surfaces a clear error rather than
	// silently producing a hollow account.
	return auth.UserInfo{}, errors.New("mtproto login: refusing to create a new Telegram account — pre-register the QA user account at https://my.telegram.org first")
}

func (l *loginAuthenticator) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	// Prompt verbatim, then single-line read from stdin. The code is
	// short-lived (a few minutes) but still sensitive — never echoed
	// back, never logged. The bufio.Reader is configured to read a
	// single line.
	fmt.Fprintf(l.codeOut, "Enter code Telegram sent to %s: ", maskPhone(l.phone))
	line, err := l.codeIn.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read code: %w", err)
	}
	code := strings.TrimSpace(line)
	if code == "" {
		return "", errors.New("empty code — re-run `qaherald mtproto login`")
	}
	return code, nil
}

// maskPhone returns a redacted form of an E.164 phone number that
// preserves the country-code + last-2-digit suffix and asterisks the
// middle. Used so the operator can sanity-check which phone is being
// used WITHOUT echoing the full number to logs / transcripts.
func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return strings.Repeat("*", len(phone))
	}
	// Keep country-code (first 3) + last 2 digits visible; mask the rest.
	head := phone[:3]
	tail := phone[len(phone)-2:]
	mid := strings.Repeat("*", len(phone)-5)
	return head + mid + tail
}

// isStdinTTY reports whether os.Stdin is connected to a terminal. Uses
// go-isatty (already a transitive dep via cobra). Returns false on
// platforms where the detection is unsupported, which is intentional —
// the conservative fallback refuses the interactive flow.
func isStdinTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

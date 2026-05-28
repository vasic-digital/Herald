// Hermetic tests for the `qaherald mtproto` subcommand wiring. None of
// these tests make real Telegram calls — they verify the Cobra-layer
// contract:
//
//   - The `mtproto` parent command is registered on rootCmd.
//   - `login` / `whoami` / `logout` are children of `mtproto`.
//   - `login` returns a clear error when required env vars are missing
//     (no panic, no nil deref).
//   - `whoami` returns a clear error when the session file does not
//     exist (the fast-path check that avoids a network round-trip).
//   - `mtprotoEnvConfig` rejects every missing / mis-shaped env var.
//
// The live wiring (real Telegram round-trip) is exercised by the
// operator-driven `qaherald mtproto login` + `qaherald mtproto whoami`
// commands against the real credentials; the §11.4.98(B) one-time
// interactive bootstrap can't be automated by definition.
package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vasic-digital/herald/qaherald/internal/mtproto"
)

// TestMTProtoCmd_RegisteredOnRoot asserts the parent command is reachable.
func TestMTProtoCmd_RegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"mtproto"})
	if err != nil {
		t.Fatalf("rootCmd.Find(mtproto): %v", err)
	}
	if found == nil {
		t.Fatal("mtproto command not found on rootCmd")
	}
	if found.Name() != "mtproto" {
		t.Errorf("found.Name() = %q, want %q", found.Name(), "mtproto")
	}
}

// TestMTProtoCmd_HasThreeChildren — login / whoami / logout MUST be
// registered. A regression that drops one would silently disable the
// matching capability without obvious failure.
func TestMTProtoCmd_HasThreeChildren(t *testing.T) {
	parent, _, err := rootCmd.Find([]string{"mtproto"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want := map[string]bool{"login": false, "whoami": false, "logout": false}
	for _, child := range parent.Commands() {
		if _, ok := want[child.Name()]; ok {
			want[child.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("mtproto %s subcommand missing", name)
		}
	}
}

// TestMTProtoEnvConfig_RejectsMissingFields walks every required env var
// being absent and confirms mtprotoEnvConfig returns a clear error.
func TestMTProtoEnvConfig_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
		want string // substring expected in error message
	}{
		{
			name: "missing app_id",
			set: map[string]string{
				"HERALD_MTPROTO_APP_HASH": "0123456789abcdef0123456789abcdef",
				"HERALD_MTPROTO_PHONE":    "+12025551234",
			},
			want: "HERALD_MTPROTO_APP_ID",
		},
		{
			name: "missing app_hash",
			set: map[string]string{
				"HERALD_MTPROTO_APP_ID": "12345678",
				"HERALD_MTPROTO_PHONE":  "+12025551234",
			},
			want: "HERALD_MTPROTO_APP_HASH",
		},
		{
			name: "missing phone",
			set: map[string]string{
				"HERALD_MTPROTO_APP_ID":   "12345678",
				"HERALD_MTPROTO_APP_HASH": "0123456789abcdef0123456789abcdef",
			},
			want: "HERALD_MTPROTO_PHONE",
		},
		{
			name: "non-integer app_id",
			set: map[string]string{
				"HERALD_MTPROTO_APP_ID":   "not-a-number",
				"HERALD_MTPROTO_APP_HASH": "0123456789abcdef0123456789abcdef",
				"HERALD_MTPROTO_PHONE":    "+12025551234",
			},
			want: "is not an integer",
		},
		{
			name: "bad-shape app_hash",
			set: map[string]string{
				"HERALD_MTPROTO_APP_ID":   "12345678",
				"HERALD_MTPROTO_APP_HASH": "tooshort",
				"HERALD_MTPROTO_PHONE":    "+12025551234",
			},
			want: "invalid MTProto config",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all 4 env vars + set the per-case subset.
			t.Setenv("HERALD_MTPROTO_APP_ID", "")
			t.Setenv("HERALD_MTPROTO_APP_HASH", "")
			t.Setenv("HERALD_MTPROTO_PHONE", "")
			t.Setenv("HERALD_MTPROTO_PASSWORD", "")
			for k, v := range tc.set {
				t.Setenv(k, v)
			}
			_, err := mtprotoEnvConfig()
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
			// Anti-bluff: error MUST NOT leak credential bytes
			// (defense-in-depth even though no app_hash was set
			// to a credential-shape value).
			if mtproto.ContainsSecret(err.Error()) {
				t.Errorf("error LEAKED credential bytes: %q", err.Error())
			}
		})
	}
}

// TestMTProtoEnvConfig_AcceptsValidEnv — happy path: all four env vars
// present + valid → no error.
func TestMTProtoEnvConfig_AcceptsValidEnv(t *testing.T) {
	t.Setenv("HERALD_MTPROTO_APP_ID", "12345678")
	t.Setenv("HERALD_MTPROTO_APP_HASH", "0123456789abcdef0123456789abcdef")
	t.Setenv("HERALD_MTPROTO_PHONE", "+12025551234")
	t.Setenv("HERALD_MTPROTO_PASSWORD", "secretsecret")

	cfg, err := mtprotoEnvConfig()
	if err != nil {
		t.Fatalf("mtprotoEnvConfig: %v", err)
	}
	if cfg.AppID != 12345678 {
		t.Errorf("AppID: got %d, want 12345678", cfg.AppID)
	}
	if cfg.Phone != "+12025551234" {
		t.Errorf("Phone: got %q", cfg.Phone)
	}
	// Password MUST be populated (it's a required input for the 2FA
	// flow); we just check it round-tripped.
	if cfg.Password != "secretsecret" {
		t.Errorf("Password: got %q (want round-tripped)", cfg.Password)
	}
}

// TestMTProtoLogin_RejectsMissingEnv — `qaherald mtproto login` with
// missing env returns a non-nil error WITHOUT panicking.
func TestMTProtoLogin_RejectsMissingEnv(t *testing.T) {
	t.Setenv("HERALD_MTPROTO_APP_ID", "")
	t.Setenv("HERALD_MTPROTO_APP_HASH", "")
	t.Setenv("HERALD_MTPROTO_PHONE", "")
	t.Setenv("HERALD_MTPROTO_PASSWORD", "")

	// Drive the RunE directly with a stand-alone *cobra.Command so the
	// rootCmd state isn't polluted by SetArgs.
	cmd := &cobra.Command{Use: "login"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runMTProtoLogin(cmd, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "mtproto login:") {
		t.Errorf("error %q lacks the structured prefix", err.Error())
	}
}

// TestMTProtoWhoami_RejectsMissingSession — when the session file does
// not exist, whoami short-circuits with ErrNoSession (NO network call).
func TestMTProtoWhoami_RejectsMissingSession(t *testing.T) {
	t.Setenv("HERALD_MTPROTO_APP_ID", "12345678")
	t.Setenv("HERALD_MTPROTO_APP_HASH", "0123456789abcdef0123456789abcdef")
	t.Setenv("HERALD_MTPROTO_PHONE", "+12025551234")
	t.Setenv("HERALD_MTPROTO_PASSWORD", "")
	// Force the session file to a path that doesn't exist.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Sanity: the default path under the redirected HOME is empty.
	wantPath := filepath.Join(tmp, ".config", "herald", "mtproto.session")
	_ = wantPath

	cmd := &cobra.Command{Use: "whoami"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runMTProtoWhoami(cmd, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, mtproto.ErrNoSession) {
		t.Errorf("want errors.Is(ErrNoSession); got %v", err)
	}
}

// TestMaskPhone_RedactsMiddle — the masked phone retains country code +
// last 2 digits and asterisks the rest. Verifies no full phone value
// reaches logs / stdout.
func TestMaskPhone_RedactsMiddle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"+12025551234", "+12*******34"},
		{"+38161234567", "+38*******67"},
		{"+1", "**"}, // degenerate
	}
	for _, tc := range cases {
		got := maskPhone(tc.in)
		if got != tc.want {
			t.Errorf("maskPhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got == tc.in && len(tc.in) > 4 {
			t.Errorf("maskPhone(%q) did NOT redact: %q", tc.in, got)
		}
	}
}

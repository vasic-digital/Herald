package mtproto

import (
	"errors"
	"os"
	"path/filepath"
)

// Session file management for the persisted MTProto auth state. After the
// one-time `qaherald mtproto login` bootstrap, the session is written to
// DefaultSessionFile() (or Config.SessionFile if non-empty), chmod 600.
// Every subsequent test invocation reads this file to authenticate without
// any human action — that's the §11.4.98(B) permitted exception working
// as designed.

// DefaultSessionFile returns ~/.config/herald/mtproto.session resolved
// against $HOME. On unusual platforms where $HOME is unset, returns
// ./mtproto.session as a last-resort fallback (the test harness sets HOME
// in t.TempDir scenarios, so the fallback is only hit in degraded shells).
func DefaultSessionFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "mtproto.session"
	}
	return filepath.Join(home, ".config", "herald", "mtproto.session")
}

// ResolvedSessionFile returns Config.SessionFile if non-empty, else
// DefaultSessionFile(). Pure helper — no I/O.
func (c *Config) ResolvedSessionFile() string {
	if c.SessionFile != "" {
		return c.SessionFile
	}
	return DefaultSessionFile()
}

// SessionExists reports whether a session file is present at the resolved
// path. Does NOT validate the content — Connect surfaces gotd/td's auth
// failure if the file is corrupt. Returns (false, nil) for missing file;
// (false, err) for unexpected errors (permission denied, etc).
func (c *Config) SessionExists() (bool, error) {
	path := c.ResolvedSessionFile()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, sanitizeMTProtoError(err)
	}
	if info.IsDir() {
		return false, sanitizeMTProtoError(errors.New("session path is a directory, not a file: " + path))
	}
	return true, nil
}

// EnsureSessionDir creates the parent directory of the session file with
// mode 0700 (owner-only). Called by the login bootstrap before writing
// the session.
func (c *Config) EnsureSessionDir() error {
	path := c.ResolvedSessionFile()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return sanitizeMTProtoError(err)
	}
	return nil
}

package channels

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrUnknownChannel is returned by New when no constructor is registered for
// the requested channel name. Callers (pherald listen) MUST surface it
// explicitly — a silent no-op channel is a §107 PASS-bluff.
var ErrUnknownChannel = errors.New("channels: unknown channel")

// Config is the per-channel constructor input — channel-agnostic; an adapter
// reads only the fields it understands.
type Config struct {
	Channel  string            // name being constructed ("tgram", "slack")
	Token    string            // primary credential (Telegram token, Slack xoxb-)
	AppToken string            // secondary (Slack xapp- app-level token for Socket Mode)
	Target   string            // default outbound dest (Telegram chat_id, Slack channel id, email)
	BaseURL  string            // httptest seam; "" => live endpoint
	Extra    map[string]string // channel-specific (e.g. email IMAP host/port)
}

// Constructor builds a Channel from a Config. Registered via init().
type Constructor func(cfg Config) (Channel, error)

var (
	mu       sync.RWMutex
	registry = map[string]Constructor{}
)

// Register installs ctor under name. Panics on duplicate — a typo in two
// adapters' init() would silently shadow one (a bluff class this framework
// forbids; mirrors qaherald scenario.Register).
func Register(name string, ctor Constructor) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("channels: duplicate registration for %q", name))
	}
	registry[name] = ctor
}

// New constructs the channel registered under name (wrapped ErrUnknownChannel
// when none).
func New(name string, cfg Config) (Channel, error) {
	mu.RLock()
	ctor, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q (registered: %v)", ErrUnknownChannel, name, Names())
	}
	cfg.Channel = name
	return ctor(cfg)
}

// Names returns every registered channel name, alphabetical.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

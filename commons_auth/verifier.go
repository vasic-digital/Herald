// Package commons_auth provides JWT verification (HMAC + JWKS modes)
// and a Gin middleware factory for Herald flavor binaries. Per Wave 3
// design §4, every serving flavor's main.go threads GinMiddleware into
// cli.ServeOpts.Middleware to gate every route.
//
// Per §11.4.74 catalogue-check: no existing vasic-digital/HelixDevelopment
// module satisfies this shape; vendored as Herald-internal. See
// docs/catalogue-checks/HRD-093-commons-auth.md.
package commons_auth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/redis/go-redis/v9"
)

// Verifier validates a JWT and returns its claims. Single method;
// concrete implementations are hmacVerifier (HS256) and jwksVerifier
// (RS256/ES256 with Redis-cached keys).
type Verifier interface {
	Verify(token string) (map[string]any, error)
}

// Mode selects which Verifier implementation NewVerifier returns.
type Mode string

const (
	// ModeHMAC uses a pre-shared secret with HS256.
	ModeHMAC Mode = "hmac"
	// ModeJWKS fetches public keys from a JWKS URL (RS256/ES256).
	ModeJWKS Mode = "jwks"
)

// Config is the runtime auth config. Populate via NewVerifierFromEnv
// or construct manually for tests.
type Config struct {
	Mode           Mode
	HMACSecret     []byte
	JWKSURL        string
	JWKSCacheTTL   time.Duration
	RequiredClaims []string
	Clock          clockwork.Clock
}

// Default required claims if none specified.
var defaultRequiredClaims = []string{"tenant", "sub"}

// NewVerifier returns a Verifier per cfg.Mode. Redis is required for
// JWKS-mode key caching; HMAC mode ignores it. When rdb is nil in JWKS
// mode the verifier falls back to in-memory-only caching.
func NewVerifier(cfg Config, rdb redis.Cmdable) (Verifier, error) {
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	if len(cfg.RequiredClaims) == 0 {
		cfg.RequiredClaims = defaultRequiredClaims
	}
	switch cfg.Mode {
	case ModeHMAC:
		if len(cfg.HMACSecret) == 0 {
			return nil, errors.New("commons_auth: HMAC mode requires HMACSecret")
		}
		return &hmacVerifier{cfg: cfg}, nil
	case ModeJWKS:
		if cfg.JWKSURL == "" {
			return nil, errors.New("commons_auth: JWKS mode requires JWKSURL")
		}
		if cfg.JWKSCacheTTL == 0 {
			cfg.JWKSCacheTTL = 5 * time.Minute
		}
		return newJWKSVerifier(cfg, rdb)
	default:
		return nil, fmt.Errorf("commons_auth: unknown mode %q", cfg.Mode)
	}
}

// NewVerifierFromEnv builds a Verifier from the documented env vars.
// HERALD_AUTH_MODE selects the mode; mode-specific vars provide secrets.
//
// Recognised env vars:
//
//	HERALD_AUTH_MODE         = "hmac" | "jwks"
//	HERALD_AUTH_HMAC_SECRET  (hmac mode) raw secret bytes
//	HERALD_AUTH_JWKS_URL     (jwks mode) HTTPS JWKS endpoint
//	HERALD_AUTH_JWKS_TTL     (jwks mode) cache TTL; Go duration ("5m")
//	                         or bare seconds ("300")
func NewVerifierFromEnv(rdb redis.Cmdable) (Verifier, error) {
	mode := Mode(os.Getenv("HERALD_AUTH_MODE"))
	if mode == "" {
		return nil, errors.New("HERALD_AUTH_MODE must be set ('hmac' or 'jwks')")
	}
	cfg := Config{Mode: mode}
	switch mode {
	case ModeHMAC:
		cfg.HMACSecret = []byte(os.Getenv("HERALD_AUTH_HMAC_SECRET"))
	case ModeJWKS:
		cfg.JWKSURL = os.Getenv("HERALD_AUTH_JWKS_URL")
		if s := os.Getenv("HERALD_AUTH_JWKS_TTL"); s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				if sec, perr := strconv.Atoi(s); perr == nil {
					d = time.Duration(sec) * time.Second
				} else {
					return nil, fmt.Errorf("HERALD_AUTH_JWKS_TTL invalid duration: %v", err)
				}
			}
			cfg.JWKSCacheTTL = d
		}
	}
	return NewVerifier(cfg, rdb)
}

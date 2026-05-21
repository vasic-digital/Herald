package commons_auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// hmacVerifier implements Verifier with HS256 + pre-shared secret.
type hmacVerifier struct {
	cfg Config
}

func (h *hmacVerifier) Verify(token string) (map[string]any, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method %s", t.Method.Alg())
		}
		return h.cfg.HMACSecret, nil
	}, jwt.WithTimeFunc(h.cfg.Clock.Now))
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("claims are not jwt.MapClaims")
	}
	out := map[string]any(claims)
	if err := requireClaims(out, h.cfg.RequiredClaims); err != nil {
		return nil, err
	}
	return out, nil
}

// requireClaims asserts every key in required exists in claims and, when
// present as a string, is non-empty. Non-string types pass through (e.g.
// numeric exp); only the presence + non-emptiness check is enforced.
func requireClaims(claims map[string]any, required []string) error {
	for _, key := range required {
		v, ok := claims[key]
		if !ok {
			return fmt.Errorf("token missing claim %q", key)
		}
		if s, isStr := v.(string); isStr && s == "" {
			return fmt.Errorf("token claim %q is empty", key)
		}
	}
	return nil
}

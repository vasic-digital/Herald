package commons_auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// jwksVerifier implements Verifier with RS256/ES256 + JWKS-fetched keys.
// Keys are cached in Redis (when available) keyed on the JWKS URL hash;
// the in-memory map is the hot path. On unknown-kid the verifier forces
// one re-fetch to handle key-rotation races.
type jwksVerifier struct {
	cfg          Config
	rdb          redis.Cmdable
	cacheKey     string
	mu           sync.RWMutex
	inMemoryKeys map[string]any // kid → *rsa.PublicKey | *ecdsa.PublicKey
	fetchedAt    time.Time
}

func newJWKSVerifier(cfg Config, rdb redis.Cmdable) (*jwksVerifier, error) {
	sum := sha256.Sum256([]byte(cfg.JWKSURL))
	return &jwksVerifier{
		cfg:          cfg,
		rdb:          rdb,
		cacheKey:     "herald:auth:jwks:" + hex.EncodeToString(sum[:]),
		inMemoryKeys: map[string]any{},
	}, nil
}

func (j *jwksVerifier) Verify(token string) (map[string]any, error) {
	parsed, err := jwt.Parse(token, j.keyFunc, jwt.WithTimeFunc(j.cfg.Clock.Now))
	if err != nil && errors.Is(err, errKidNotFound) {
		// Force re-fetch once for rotation-race.
		if rerr := j.refresh(context.Background(), true); rerr != nil {
			return nil, fmt.Errorf("invalid token: %w (refresh failed: %v)", err, rerr)
		}
		parsed, err = jwt.Parse(token, j.keyFunc, jwt.WithTimeFunc(j.cfg.Clock.Now))
	}
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
	if err := requireClaims(out, j.cfg.RequiredClaims); err != nil {
		return nil, err
	}
	return out, nil
}

var errKidNotFound = errors.New("kid not in cache")

func (j *jwksVerifier) keyFunc(t *jwt.Token) (any, error) {
	// Restrict to the algorithms we explicitly support — anything else is
	// either spec-violating (alg: none) or out-of-scope.
	switch t.Method.Alg() {
	case jwt.SigningMethodRS256.Alg(),
		jwt.SigningMethodRS384.Alg(),
		jwt.SigningMethodRS512.Alg(),
		jwt.SigningMethodES256.Alg(),
		jwt.SigningMethodES384.Alg(),
		jwt.SigningMethodES512.Alg():
		// supported
	default:
		return nil, fmt.Errorf("unexpected signing method %s", t.Method.Alg())
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("token missing kid")
	}
	j.mu.RLock()
	if k, ok := j.inMemoryKeys[kid]; ok {
		j.mu.RUnlock()
		return k, nil
	}
	j.mu.RUnlock()
	if err := j.refresh(context.Background(), false); err != nil {
		return nil, err
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if k, ok := j.inMemoryKeys[kid]; ok {
		return k, nil
	}
	return nil, errKidNotFound
}

// refresh fetches JWKS — checks Redis cache first (unless force=true);
// on miss/expiry fetches HTTPS and updates Redis + in-memory.
// Falls back to in-memory-only when Redis is unreachable.
func (j *jwksVerifier) refresh(ctx context.Context, force bool) error {
	if !force && j.rdb != nil {
		val, err := j.rdb.Get(ctx, j.cacheKey).Bytes()
		if err == nil && len(val) > 0 {
			return j.loadJWKS(val)
		}
		// continue to HTTPS fetch on cache miss / Redis error
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.cfg.JWKSURL, nil)
	if err != nil {
		return fmt.Errorf("build JWKS request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read JWKS: %w", err)
	}
	if j.rdb != nil {
		// Redis errors are non-fatal — keys are still loaded into memory.
		_ = j.rdb.Set(ctx, j.cacheKey, body, j.cfg.JWKSCacheTTL).Err()
	}
	return j.loadJWKS(body)
}

// loadJWKS parses a JWKS JSON document and populates inMemoryKeys.
// Supports RSA (RS256/RS384/RS512) and EC (ES256/ES384/ES512) per RFC 7517.
func (j *jwksVerifier) loadJWKS(raw []byte) error {
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}
	parsed, err := parseJWKSKeys(doc.Keys)
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.inMemoryKeys = parsed
	j.fetchedAt = j.cfg.Clock.Now()
	j.mu.Unlock()
	return nil
}

// parseJWKSKeys converts a list of JWK maps into kid → public key entries
// per RFC 7517. Supported key types:
//
//   - kty=RSA  with n+e (RFC 7518 §6.3.1) → *rsa.PublicKey
//   - kty=EC   with crv+x+y (RFC 7518 §6.2.1) → *ecdsa.PublicKey, crv in
//     {P-256, P-384, P-521}
//
// Unsupported kty values (e.g. oct, OKP/Ed25519) are skipped silently so
// JWKS documents containing extra key types do not fail the load; if no
// keys survive parsing the function returns an error.
//
// Each key MUST have a non-empty kid (RFC 7517 §4.5) — kid is the lookup
// index in inMemoryKeys.
func parseJWKSKeys(keys []map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(keys))
	for i, k := range keys {
		kid, _ := k["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("JWK at index %d missing kid", i)
		}
		kty, _ := k["kty"].(string)
		switch kty {
		case "RSA":
			pub, err := parseRSAJWK(k)
			if err != nil {
				return nil, fmt.Errorf("JWK %q (RSA): %w", kid, err)
			}
			out[kid] = pub
		case "EC":
			pub, err := parseECJWK(k)
			if err != nil {
				return nil, fmt.Errorf("JWK %q (EC): %w", kid, err)
			}
			out[kid] = pub
		case "":
			return nil, fmt.Errorf("JWK %q missing kty", kid)
		default:
			// Skip unsupported kty (oct symmetric, OKP/Ed25519) — explicit
			// per RFC 7517 §6: parsers MAY ignore unrecognised keys.
			continue
		}
	}
	if len(out) == 0 {
		return nil, errors.New("JWKS document contained no supported keys")
	}
	return out, nil
}

// parseRSAJWK builds an *rsa.PublicKey from the n + e base64url members
// per RFC 7518 §6.3.1.
func parseRSAJWK(k map[string]any) (*rsa.PublicKey, error) {
	nStr, _ := k["n"].(string)
	eStr, _ := k["e"].(string)
	if nStr == "" || eStr == "" {
		return nil, errors.New("missing n or e")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	if len(nBytes) == 0 {
		return nil, errors.New("n decoded to zero bytes")
	}
	if len(eBytes) == 0 {
		return nil, errors.New("e decoded to zero bytes")
	}
	eInt := new(big.Int).SetBytes(eBytes)
	if !eInt.IsInt64() || eInt.Int64() < 1 {
		return nil, fmt.Errorf("invalid exponent value %s", eInt.String())
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(eInt.Int64()),
	}, nil
}

// parseECJWK builds an *ecdsa.PublicKey from the crv + x + y base64url
// members per RFC 7518 §6.2.1.
func parseECJWK(k map[string]any) (*ecdsa.PublicKey, error) {
	crv, _ := k["crv"].(string)
	xStr, _ := k["x"].(string)
	yStr, _ := k["y"].(string)
	if xStr == "" || yStr == "" {
		return nil, errors.New("missing x or y")
	}
	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve %q", crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(xStr)
	if err != nil {
		return nil, fmt.Errorf("decode x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yStr)
	if err != nil {
		return nil, fmt.Errorf("decode y: %w", err)
	}
	pub := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	if !curve.IsOnCurve(pub.X, pub.Y) {
		return nil, errors.New("EC point not on curve")
	}
	return pub, nil
}

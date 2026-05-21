package commons_auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/vasic-digital/herald/commons_auth"
)

func TestHMACVerifier_VerifiesValidToken(t *testing.T) {
	secret := []byte("test-secret-32-bytes-of-padding!!")
	cfg := commons_auth.Config{
		Mode:           commons_auth.ModeHMAC,
		HMACSecret:     secret,
		RequiredClaims: []string{"tenant", "sub"},
	}
	v, err := commons_auth.NewVerifier(cfg, nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "operator@example.com",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	claims, err := v.Verify(signed)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if claims["tenant"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("tenant claim missing or wrong: %v", claims["tenant"])
	}
	if claims["sub"] != "operator@example.com" {
		t.Errorf("sub claim missing or wrong: %v", claims["sub"])
	}
}

func TestHMACVerifier_RejectsWrongSecret(t *testing.T) {
	secret := []byte("good-secret-32-bytes-of-padding!!")
	other := []byte("evil-secret-32-bytes-of-padding!!")
	cfg := commons_auth.Config{Mode: commons_auth.ModeHMAC, HMACSecret: secret, RequiredClaims: []string{"tenant"}}
	v, _ := commons_auth.NewVerifier(cfg, nil)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "x",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	signed, _ := tok.SignedString(other) // signed with different secret
	if _, err := v.Verify(signed); err == nil {
		t.Fatal("expected error for wrong-signature token")
	}
}

func TestHMACVerifier_RejectsMissingRequiredClaim(t *testing.T) {
	secret := []byte("test-secret-32-bytes-of-padding!!")
	cfg := commons_auth.Config{Mode: commons_auth.ModeHMAC, HMACSecret: secret, RequiredClaims: []string{"tenant", "sub"}}
	v, _ := commons_auth.NewVerifier(cfg, nil)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		// missing tenant claim
		"sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, _ := tok.SignedString(secret)
	if _, err := v.Verify(signed); err == nil {
		t.Fatal("expected error for missing tenant claim")
	}
}

func TestHMACVerifier_RejectsExpiredToken(t *testing.T) {
	secret := []byte("test-secret-32-bytes-of-padding!!")
	cfg := commons_auth.Config{Mode: commons_auth.ModeHMAC, HMACSecret: secret, RequiredClaims: []string{"tenant"}}
	v, _ := commons_auth.NewVerifier(cfg, nil)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "x",
		"exp":    time.Now().Add(-time.Hour).Unix(), // expired 1h ago
	})
	signed, _ := tok.SignedString(secret)
	if _, err := v.Verify(signed); err == nil {
		t.Fatal("expected error for expired token")
	}
}

// TestJWKSVerifier_VerifiesTokenSignedByPubInJWKS builds a real RSA key,
// publishes its public component as a JWKS document via httptest, signs a
// token with the matching private key, and asserts the verifier accepts it.
// This is the §107 anti-bluff proof that parseJWKSKeys + RS256 verification
// actually work end-to-end through the public API.
func TestJWKSVerifier_VerifiesTokenSignedByPubInJWKS(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	const kid = "test-kid-1"

	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": kid,
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
			},
		},
	}
	body, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cfg := commons_auth.Config{
		Mode:           commons_auth.ModeJWKS,
		JWKSURL:        srv.URL,
		JWKSCacheTTL:   1 * time.Minute,
		RequiredClaims: []string{"tenant", "sub"},
	}
	// rdb=nil exercises the in-memory-only fallback path.
	v, err := commons_auth.NewVerifier(cfg, nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "operator@example.com",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	claims, err := v.Verify(signed)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if claims["tenant"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("tenant claim missing or wrong: %v", claims["tenant"])
	}
	if claims["sub"] != "operator@example.com" {
		t.Errorf("sub claim missing or wrong: %v", claims["sub"])
	}
}

// TestGinMiddleware_EndToEnd is the §107 anti-bluff smoke. It boots a real
// Gin server gated by GinMiddleware(v) on a real httptest listener, signs a
// real HMAC token, sends a real Authorization header through the HTTP client,
// and asserts the downstream handler observes the claims map under
// ContextKeyClaims. This proves the public API (NewVerifier + GinMiddleware
// + ContextKeyClaims) actually works end-to-end — not just unit-isolated.
func TestGinMiddleware_EndToEnd(t *testing.T) {
	secret := []byte("e2e-secret-32-bytes-of-padding!!!")
	v, err := commons_auth.NewVerifier(commons_auth.Config{
		Mode:           commons_auth.ModeHMAC,
		HMACSecret:     secret,
		RequiredClaims: []string{"tenant", "sub"},
	}, nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(commons_auth.GinMiddleware(v))
	r.GET("/ping", func(c *gin.Context) {
		raw, ok := c.Get(commons_auth.ContextKeyClaims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no claims in ctx"})
			return
		}
		claims, ok := raw.(map[string]any)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "claims wrong type"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tenant": claims["tenant"],
			"sub":    claims["sub"],
		})
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "smoke@example.com",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	// Positive: valid bearer → 200 + claims roundtrip.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+signed)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "550e8400-e29b-41d4-a716-446655440000") {
		t.Errorf("response body missing tenant: %s", body)
	}
	if !strings.Contains(string(body), "smoke@example.com") {
		t.Errorf("response body missing sub: %s", body)
	}

	// Negative: missing Authorization → 401.
	reqMissing, _ := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	respMissing, err := http.DefaultClient.Do(reqMissing)
	if err != nil {
		t.Fatalf("Do(missing): %v", err)
	}
	defer respMissing.Body.Close()
	if respMissing.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token, got %d", respMissing.StatusCode)
	}

	// Negative: wrong-scheme header → 401.
	reqBad, _ := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	reqBad.Header.Set("Authorization", "Basic "+signed)
	respBad, err := http.DefaultClient.Do(reqBad)
	if err != nil {
		t.Fatalf("Do(bad-scheme): %v", err)
	}
	defer respBad.Body.Close()
	if respBad.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong scheme, got %d", respBad.StatusCode)
	}

	// Negative: invalid token → 401.
	reqInvalid, _ := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	reqInvalid.Header.Set("Authorization", "Bearer not-a-real-jwt")
	respInvalid, err := http.DefaultClient.Do(reqInvalid)
	if err != nil {
		t.Fatalf("Do(invalid): %v", err)
	}
	defer respInvalid.Body.Close()
	if respInvalid.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", respInvalid.StatusCode)
	}
}

// TestTenantAndSubjectFromClaims exercises the claims helpers explicitly so
// the public surface used by downstream handlers is positively asserted.
func TestTenantAndSubjectFromClaims(t *testing.T) {
	claims := map[string]any{
		"tenant": "550e8400-e29b-41d4-a716-446655440000",
		"sub":    "ops@example.com",
	}
	tid, err := commons_auth.TenantFromClaims(claims)
	if err != nil {
		t.Fatalf("TenantFromClaims: %v", err)
	}
	if tid.String() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("tenant parse mismatch: %s", tid)
	}
	sub, err := commons_auth.SubjectFromClaims(claims)
	if err != nil {
		t.Fatalf("SubjectFromClaims: %v", err)
	}
	if sub != "ops@example.com" {
		t.Errorf("sub mismatch: %q", sub)
	}

	if _, err := commons_auth.TenantFromClaims(map[string]any{}); err == nil {
		t.Error("expected error for missing tenant claim")
	}
	if _, err := commons_auth.TenantFromClaims(map[string]any{"tenant": 42}); err == nil {
		t.Error("expected error for non-string tenant claim")
	}
	if _, err := commons_auth.SubjectFromClaims(map[string]any{}); err == nil {
		t.Error("expected error for missing sub claim")
	}
}

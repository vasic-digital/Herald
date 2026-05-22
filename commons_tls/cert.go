// Package commons_tls is Herald's TLS cert-sourcing helper.
//
// Two entry points:
//
//	LoadOrGenerate(certPath, keyPath) — Loads a tls.Certificate if BOTH
//	    files exist on disk. If NEITHER file exists, generates a fresh
//	    ECDSA P-256 self-signed certificate with SAN list
//	    [localhost, 127.0.0.1, ::1] valid for 365 days, persists the
//	    cert at certPath (mode 0644) and the key at keyPath (mode 0600),
//	    and returns the loaded pair. If EXACTLY ONE of the two files
//	    exists, returns an error — Herald refuses to auto-pair a
//	    user-supplied cert with an auto-generated key (or vice versa)
//	    because the asymmetric configuration is almost certainly an
//	    operator typo + auto-pairing would silently bind a key the
//	    operator did not authorize.
//
//	ResolveCertSource(cfg Config) — Herald's full TLS cert resolution
//	    policy. Returns a CertSource describing whether the cert came
//	    from an operator flag, env var, or dev-autogen. Production mode
//	    (cfg.ProdMode true; caller's convention: HERALD_AUTH_MODE=jwks)
//	    fails loud when no explicit cert/key path is supplied — there
//	    is NO silent dev-autogen fallback in production.
//
// Per §107: a "TLS configured" PASS without observing a real handshake
// + the actual key material on disk is a §11.4 bluff. The accompanying
// unit tests parse the generated x509.Certificate, assert
// ECDSAWithSHA256 + curve P-256 + the full SAN list, and stat() the key
// file to confirm permission 0600 — these are the load-bearing positive
// evidence that the generator does what it claims.
package commons_tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// devSANs is the SAN list baked into auto-generated dev certs. Covers
// loopback testing on dual-stack hosts (IPv4 + IPv6) plus "localhost"
// for clients that resolve via name.
var devSANs = []string{"localhost", "127.0.0.1", "::1"}

// devValidity is the validity window for auto-generated dev certs.
// 365 days is long enough to survive most active dev cycles; renew by
// deleting ~/.herald/dev-{cert,key}.pem and restarting the flavor.
const devValidity = 365 * 24 * time.Hour

// Config holds inputs to ResolveCertSource.
//
// CertPath + KeyPath: when both are set, ResolveCertSource loads them
// directly and tags the source as "flag". When both empty, the env
// (HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH) is consulted; if both
// env vars are set the source is tagged "env". Otherwise dev-autogen
// runs (in dev mode) or the call fails (in production).
//
// ProdMode true ⇒ production deployment signal. ResolveCertSource will
// REFUSE to dev-autogen and instead return an error directing the
// operator to supply --tls-cert / --tls-key. Herald convention: the
// caller sets ProdMode = (os.Getenv("HERALD_AUTH_MODE") == "jwks").
type Config struct {
	CertPath string
	KeyPath  string
	ProdMode bool
}

// CertSource is the return value of ResolveCertSource. The Source
// string is one of:
//
//	"flag"        — both Config.CertPath + Config.KeyPath supplied.
//	"env"         — both HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH set.
//	"autogen-dev" — dev mode + neither flag nor env supplied; cert was
//	                loaded from ~/.herald/dev-{cert,key}.pem (generated
//	                on first call, reused thereafter).
type CertSource struct {
	Cert   *tls.Certificate
	Source string
}

// LoadOrGenerate loads a TLS certificate pair from disk if BOTH files
// exist, or generates a fresh ECDSA P-256 self-signed cert (SAN list
// [localhost, 127.0.0.1, ::1]; validity 365 days) when NEITHER file
// exists. The key file is persisted with permission 0600.
//
// Asymmetric configurations — EXACTLY ONE of cert/key on disk — return
// an error. Herald refuses to auto-pair operator-supplied material
// with auto-generated material because the asymmetric state is almost
// certainly an operator typo, and silently auto-pairing would bind a
// key the operator did not authorize.
func LoadOrGenerate(certPath, keyPath string) (*tls.Certificate, error) {
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	certExists := certErr == nil
	keyExists := keyErr == nil

	switch {
	case certExists && keyExists:
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("commons_tls: load existing cert/key pair: %w", err)
		}
		return &cert, nil
	case certExists != keyExists:
		var present, missing string
		if certExists {
			present, missing = certPath, keyPath
		} else {
			present, missing = keyPath, certPath
		}
		return nil, fmt.Errorf("commons_tls: asymmetric cert/key configuration — %s exists but %s does not; refusing to auto-pair operator-supplied material with auto-generated material (likely operator typo)", present, missing)
	}

	// Neither exists — generate a fresh ECDSA P-256 self-signed pair.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: generate ECDSA P-256 key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("commons_tls: serial: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Herald DEV (commons_tls auto-gen)"},
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(devValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
	}
	for _, h := range devSANs {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: create cert: %w", err)
	}

	// Ensure the destination directory exists (mode 0700; we own keys).
	if dir := filepath.Dir(certPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("commons_tls: mkdir cert dir: %w", err)
		}
	}
	if dir := filepath.Dir(keyPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("commons_tls: mkdir key dir: %w", err)
		}
	}

	// Persist the cert (0644 — public material).
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: open cert file: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certOut.Close()
		return nil, fmt.Errorf("commons_tls: write cert: %w", err)
	}
	if err := certOut.Close(); err != nil {
		return nil, fmt.Errorf("commons_tls: close cert file: %w", err)
	}

	// Persist the key (0600 — secret material; permission MUST be enforced).
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: marshal EC key: %w", err)
	}
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: open key file: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		keyOut.Close()
		return nil, fmt.Errorf("commons_tls: write key: %w", err)
	}
	if err := keyOut.Close(); err != nil {
		return nil, fmt.Errorf("commons_tls: close key file: %w", err)
	}

	// Defensive: re-chmod the key file in case the open() umask widened it.
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return nil, fmt.Errorf("commons_tls: chmod key file 0600: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("commons_tls: load freshly generated cert/key: %w", err)
	}
	return &cert, nil
}

// ResolveCertSource implements Herald's full TLS cert resolution policy.
//
// Precedence (high → low):
//
//  1. Operator-supplied flags (cfg.CertPath + cfg.KeyPath both set).
//  2. Operator-supplied env (HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH).
//  3. Dev auto-generation at ~/.herald/dev-{cert,key}.pem.
//  4. Error — when cfg.ProdMode is true AND none of (1)/(2) supplied.
//
// Mismatched pairs (exactly one of the two members set, on either flag
// or env) return an error — Herald refuses to silently fall through to
// a lower-precedence source when the operator clearly intended to
// supply something at the higher tier but typo'd one of the two.
//
// In production (cfg.ProdMode true) the dev-autogen path MUST NOT be
// taken. Better to fail loud than silently serve a self-signed cert in
// a production deployment.
func ResolveCertSource(cfg Config) (*CertSource, error) {
	// Tier 1: flags.
	if cfg.CertPath != "" && cfg.KeyPath != "" {
		c, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("commons_tls: load flag-supplied cert/key: %w", err)
		}
		return &CertSource{Cert: &c, Source: "flag"}, nil
	}
	if cfg.CertPath != "" || cfg.KeyPath != "" {
		return nil, errors.New("commons_tls: --tls-cert + --tls-key must be supplied together (one without the other is incomplete)")
	}

	// Tier 2: env.
	envCert := os.Getenv("HERALD_TLS_CERT_PATH")
	envKey := os.Getenv("HERALD_TLS_KEY_PATH")
	if envCert != "" && envKey != "" {
		c, err := tls.LoadX509KeyPair(envCert, envKey)
		if err != nil {
			return nil, fmt.Errorf("commons_tls: load env-supplied cert/key: %w", err)
		}
		return &CertSource{Cert: &c, Source: "env"}, nil
	}
	if envCert != "" || envKey != "" {
		return nil, errors.New("commons_tls: HERALD_TLS_CERT_PATH + HERALD_TLS_KEY_PATH must be supplied together")
	}

	// Tier 4 (checked before Tier 3): production mode requires explicit cert.
	if cfg.ProdMode {
		return nil, errors.New("commons_tls: production mode (HERALD_AUTH_MODE=jwks) requires --tls-cert + --tls-key (or HERALD_TLS_CERT_PATH/HERALD_TLS_KEY_PATH env); no dev-autogen fallback in production")
	}

	// Tier 3: dev auto-generation under ~/.herald/.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("commons_tls: resolve home dir for dev-autogen: %w", err)
	}
	devDir := filepath.Join(home, ".herald")
	cert, err := LoadOrGenerate(
		filepath.Join(devDir, "dev-cert.pem"),
		filepath.Join(devDir, "dev-key.pem"),
	)
	if err != nil {
		return nil, err
	}
	return &CertSource{Cert: cert, Source: "autogen-dev"}, nil
}

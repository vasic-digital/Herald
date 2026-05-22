package commons_tls

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadOrGenerate_BothFilesExist_LoadsThem pre-writes a cert+key pair
// (by calling LoadOrGenerate once, which generates them), then calls
// LoadOrGenerate a second time on the same paths and asserts that the
// loaded cert chain bytes are byte-identical — proving the second call
// LOADED rather than REGENERATED.
func TestLoadOrGenerate_BothFilesExist_LoadsThem(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	first, err := LoadOrGenerate(certPath, keyPath)
	if err != nil {
		t.Fatalf("first LoadOrGenerate (seed): %v", err)
	}
	if first == nil || len(first.Certificate) == 0 {
		t.Fatal("first call returned empty cert chain")
	}

	// Both files exist after the seed call — capture cert bytes for comparison.
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert file missing after seed: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("key file missing after seed: %v", err)
	}

	second, err := LoadOrGenerate(certPath, keyPath)
	if err != nil {
		t.Fatalf("second LoadOrGenerate (load): %v", err)
	}
	if second == nil || len(second.Certificate) == 0 {
		t.Fatal("second call returned empty cert chain")
	}
	if len(first.Certificate) != len(second.Certificate) {
		t.Fatalf("cert chain length differs across calls (1st=%d, 2nd=%d) — re-generated when it should have loaded",
			len(first.Certificate), len(second.Certificate))
	}
	for i := range first.Certificate {
		if string(first.Certificate[i]) != string(second.Certificate[i]) {
			t.Errorf("cert chain[%d] differs across calls — re-generated when it should have loaded", i)
		}
	}
}

// TestLoadOrGenerate_NeitherExists_AutoGenerates is the load-bearing
// §107 anti-bluff test. It asserts that the generator produces a real
// ECDSA P-256 cert with SignatureAlgorithm == ECDSAWithSHA256 and the
// configured SAN list — not just "a file appeared on disk".
func TestLoadOrGenerate_NeitherExists_AutoGenerates(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	cert, err := LoadOrGenerate(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	if cert == nil || len(cert.Certificate) == 0 {
		t.Fatal("cert.Certificate empty")
	}

	// Files persisted.
	certStat, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("cert file not persisted: %v", err)
	}
	if certStat.Size() == 0 {
		t.Fatal("cert file is empty")
	}
	keyStat, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file not persisted: %v", err)
	}
	if keyStat.Size() == 0 {
		t.Fatal("key file is empty")
	}

	// Key permission 0600 — secret material.
	if perm := keyStat.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file mode = %o, want 0600 (secret material leak risk)", perm)
	}

	// Parse the leaf cert and inspect its properties — anti-bluff
	// verification per §107. A test that only checks "file exists"
	// would pass even if the bytes were garbage.
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// SignatureAlgorithm assertion (user-spec verbatim).
	if parsed.SignatureAlgorithm != x509.ECDSAWithSHA256 {
		t.Errorf("SignatureAlgorithm = %v, want ECDSAWithSHA256", parsed.SignatureAlgorithm)
	}

	// Public key is ECDSA P-256.
	pub, ok := parsed.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("PublicKey type = %T, want *ecdsa.PublicKey", parsed.PublicKey)
	}
	if pub.Curve == nil || pub.Curve.Params().Name != "P-256" {
		curve := "<nil>"
		if pub.Curve != nil {
			curve = pub.Curve.Params().Name
		}
		t.Errorf("curve = %s, want P-256", curve)
	}

	// SAN: DNS names include "localhost".
	hasLocalhost := false
	for _, dns := range parsed.DNSNames {
		if dns == "localhost" {
			hasLocalhost = true
			break
		}
	}
	if !hasLocalhost {
		t.Errorf("DNSNames = %v, want to contain 'localhost'", parsed.DNSNames)
	}

	// SAN: IP addresses include 127.0.0.1 and ::1.
	wantIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	for _, want := range wantIPs {
		found := false
		for _, got := range parsed.IPAddresses {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("IPAddresses %v, want to contain %v", parsed.IPAddresses, want)
		}
	}

	// Sanity: the cert really did load as a usable tls.Certificate.
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Errorf("freshly generated pair fails tls.LoadX509KeyPair: %v", err)
	}
}

// TestLoadOrGenerate_OnlyOneExists_Errors asserts the asymmetric-config
// guard fires. Herald refuses to auto-pair a user-provided cert with an
// auto-generated key (or vice versa).
func TestLoadOrGenerate_OnlyOneExists_Errors(t *testing.T) {
	// Case A: only cert file exists.
	t.Run("only_cert_exists", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "cert.pem")
		keyPath := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(certPath, []byte("dummy"), 0o644); err != nil {
			t.Fatalf("seed cert file: %v", err)
		}
		_, err := LoadOrGenerate(certPath, keyPath)
		if err == nil {
			t.Fatal("LoadOrGenerate with only cert present MUST error")
		}
		if !strings.Contains(err.Error(), "asymmetric") {
			t.Errorf("error %q should mention 'asymmetric'", err.Error())
		}
	})

	// Case B: only key file exists.
	t.Run("only_key_exists", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "cert.pem")
		keyPath := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(keyPath, []byte("dummy"), 0o600); err != nil {
			t.Fatalf("seed key file: %v", err)
		}
		_, err := LoadOrGenerate(certPath, keyPath)
		if err == nil {
			t.Fatal("LoadOrGenerate with only key present MUST error")
		}
		if !strings.Contains(err.Error(), "asymmetric") {
			t.Errorf("error %q should mention 'asymmetric'", err.Error())
		}
	})
}

// TestResolveCertSource_DevModeNoFlags_AutoGen asserts that dev mode
// (cfg.ProdMode=false) with no flags + no env triggers dev-autogen at
// $HOME/.herald/, and the Source field is "autogen-dev".
func TestResolveCertSource_DevModeNoFlags_AutoGen(t *testing.T) {
	// Redirect $HOME so we don't litter the real ~/.herald during testing.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Clear env vars so the dev path is taken.
	t.Setenv("HERALD_TLS_CERT_PATH", "")
	t.Setenv("HERALD_TLS_KEY_PATH", "")

	src, err := ResolveCertSource(Config{ProdMode: false})
	if err != nil {
		t.Fatalf("ResolveCertSource dev mode: %v", err)
	}
	if src == nil || src.Cert == nil {
		t.Fatal("CertSource or Cert is nil")
	}
	if src.Source != "autogen-dev" {
		t.Errorf("Source = %q, want %q", src.Source, "autogen-dev")
	}

	// Files persisted under the redirected $HOME/.herald/.
	if _, err := os.Stat(filepath.Join(tmp, ".herald", "dev-cert.pem")); err != nil {
		t.Errorf("dev cert not persisted at $HOME/.herald/dev-cert.pem: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".herald", "dev-key.pem")); err != nil {
		t.Errorf("dev key not persisted at $HOME/.herald/dev-key.pem: %v", err)
	}
}

// TestResolveCertSource_ProdModeNoFlags_Errors is the load-bearing
// §107 anti-bluff test for the production-mode fail-loud policy. The
// test reads the actual error message and asserts it contains the
// substring "production mode" — proving the prod-mode branch actually
// fired (not some earlier validation error that happens to also fail).
func TestResolveCertSource_ProdModeNoFlags_Errors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HERALD_TLS_CERT_PATH", "")
	t.Setenv("HERALD_TLS_KEY_PATH", "")

	_, err := ResolveCertSource(Config{ProdMode: true})
	if err == nil {
		t.Fatal("ResolveCertSource in prod mode with no cert MUST error")
	}
	if !strings.Contains(err.Error(), "production mode") {
		t.Errorf("error = %q, want to contain 'production mode'", err.Error())
	}
}

// TestResolveCertSource_FlagWinsOverEnv asserts the precedence ladder:
// when both flag-supplied paths AND env-supplied paths are present, the
// flag-supplied pair wins (Source == "flag").
func TestResolveCertSource_FlagWinsOverEnv(t *testing.T) {
	// Seed two distinct cert/key pairs on disk.
	flagDir := t.TempDir()
	envDir := t.TempDir()

	flagCert := filepath.Join(flagDir, "cert.pem")
	flagKey := filepath.Join(flagDir, "key.pem")
	if _, err := LoadOrGenerate(flagCert, flagKey); err != nil {
		t.Fatalf("seed flag pair: %v", err)
	}

	envCert := filepath.Join(envDir, "cert.pem")
	envKey := filepath.Join(envDir, "key.pem")
	if _, err := LoadOrGenerate(envCert, envKey); err != nil {
		t.Fatalf("seed env pair: %v", err)
	}

	// Set env vars (would otherwise be selected at Tier 2).
	t.Setenv("HERALD_TLS_CERT_PATH", envCert)
	t.Setenv("HERALD_TLS_KEY_PATH", envKey)

	src, err := ResolveCertSource(Config{
		CertPath: flagCert,
		KeyPath:  flagKey,
		ProdMode: false,
	})
	if err != nil {
		t.Fatalf("ResolveCertSource: %v", err)
	}
	if src == nil || src.Cert == nil {
		t.Fatal("CertSource or Cert is nil")
	}
	if src.Source != "flag" {
		t.Errorf("Source = %q, want %q (flag must win over env)", src.Source, "flag")
	}

	// Cross-check: the resolved cert MUST match the flag-supplied file
	// byte-for-byte, NOT the env-supplied one. This proves the
	// precedence is real, not just label-correct.
	flagLoaded, err := tls.LoadX509KeyPair(flagCert, flagKey)
	if err != nil {
		t.Fatalf("re-load flag pair: %v", err)
	}
	if len(src.Cert.Certificate) != len(flagLoaded.Certificate) ||
		string(src.Cert.Certificate[0]) != string(flagLoaded.Certificate[0]) {
		t.Error("resolved cert bytes do not match flag-supplied cert — flag did not actually win")
	}
}

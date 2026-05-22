// Package commons — Branding factory per spec V3 §6.3 + §11.0.
package commons

// DefaultBranding returns a sensible Branding for the named flavor.
// Each <prefix>herald binary calls this at startup and threads the
// returned Branding through every OutboundMessage; channel adapters
// use it to render rich-message accents.
//
// Wave 2 r1 (2026-05-21): also populates the per-flavor identity fields
// (Flavor / Prefix / DisplayName / DefaultPort / Mission) consumed by
// the shared commons/cli/ scaffold. Per design §3.5 the DefaultPort for
// serving flavors lives in the 247xx range; CLI-only flavors set
// DefaultPort=0 to signal "no HTTP serve mode".
func DefaultBranding(flavor string, version string) Branding {
	b := Branding{
		BinaryName: flavor + "herald",
	}
	switch flavor {
	case "p":
		b.AppName = "Project Herald"
		b.AccentColorHex = "#2C7BE5"
		b.Prefix = "PHR"
		b.DisplayName = "Project Herald"
		b.DefaultPort = 24791
		b.Mission = "Multi-mirror push + submodule propagation + project bindings"
	case "s":
		b.AppName = "System Herald"
		b.AccentColorHex = "#39AFD1"
		b.Prefix = "SHR"
		b.DisplayName = "System Herald"
		b.DefaultPort = 24793
		b.Mission = "Host-level system events fan-out + safety-state guards"
	case "b":
		// Wave 2 §3.5 Mission alignment: design doc names the flavor's
		// mission as CI/test bindings + test-tier verifier + evidence
		// capture. Prefix (BHR) + AccentColorHex unchanged; DefaultPort
		// remains 0 (CLI-only — no serve).
		b.AppName = "Build Herald"
		b.AccentColorHex = "#F6C343"
		b.Prefix = "BHR"
		b.DisplayName = "Build Herald"
		b.DefaultPort = 0
		b.Mission = "CI/test bindings + test-tier verifier + evidence capture"
	case "d":
		b.AppName = "Deploy Herald"
		b.AccentColorHex = "#42BA96"
		b.Prefix = "DHR"
		b.DisplayName = "Deploy Herald"
		b.DefaultPort = 0
		b.Mission = "Deploy lifecycle event fan-out (CLI-only)"
	case "a":
		b.AppName = "Alert Herald"
		b.AccentColorHex = "#DF4759"
		b.Prefix = "AHR"
		b.DisplayName = "Alert Herald"
		b.DefaultPort = 0
		b.Mission = "Alert / paging event fan-out (CLI-only)"
	case "sc":
		// Wave 2 §3.5: rename "Schedule Herald" → "Scheduled-audit
		// Herald" and align Mission with design doc (periodic Status.md
		// sweep + compliance digest). Prefix (SCR) + AccentColorHex
		// unchanged; CLI-only (DefaultPort=0).
		b.AppName = "Scheduled-audit Herald"
		b.AccentColorHex = "#A66DD4"
		b.Prefix = "SCR"
		b.DisplayName = "Scheduled-audit Herald"
		b.DefaultPort = 0
		b.Mission = "Periodic Status.md sweep + compliance digest"
	case "i":
		// Wave 2 §3.5 Mission alignment: design doc names the flavor's
		// mission as credential-leak page-out + operator-blocked
		// escalation. Port (24794) + Prefix (IHR) + AccentColorHex
		// unchanged.
		b.AppName = "Incident Herald"
		b.AccentColorHex = "#E63757"
		b.Prefix = "IHR"
		b.DisplayName = "Incident Herald"
		b.DefaultPort = 24794
		b.Mission = "Credential-leak page-out + operator-blocked escalation"
	case "r":
		// Wave 2 §3.5: fix DisplayName typo ("RHR Herald" → "Release
		// Herald") and align Mission with design doc (tag mirroring +
		// changelog + installable-asset evidence). Prefix (RHR) +
		// AccentColorHex unchanged; CLI-only (DefaultPort=0).
		b.AppName = "Release Herald"
		b.AccentColorHex = "#6E84A3"
		b.Prefix = "RHR"
		b.DisplayName = "Release Herald"
		b.DefaultPort = 0
		b.Mission = "Tag mirroring + changelog + installable-asset evidence"
	case "c":
		// Wave 2 §3.5 rename: "Compliance Herald" → "Constitution Herald".
		// Per design doc the flavor is the policy evaluator + creds scan
		// + docs sync + composite gate. Port (24792) + Prefix (CHR) +
		// AccentColorHex unchanged.
		b.AppName = "Constitution Herald"
		b.AccentColorHex = "#283E59"
		b.Prefix = "CHR"
		b.DisplayName = "Constitution Herald"
		b.DefaultPort = 24792
		b.Mission = "Policy evaluator + creds scan + docs sync + composite gate"
	case "qa":
		// Wave 5 (operator-locked, 2026-05-22): qaherald is Herald's QA
		// bot — drives pherald ↔ Telegram round-trips end-to-end,
		// records bidirectional transcripts + sha256-checked attachments
		// under docs/qa/<run-id>/. CLI-only (DefaultPort=0 — qaherald
		// drives external services, doesn't serve HTTP itself). Two-
		// letter flavor key ("qa") matches the `qaherald` binary name's
		// 2-char prefix; mirrors the existing two-letter "sc" key for
		// scherald. Per §107.x docs/qa evidence mandate.
		b.AppName = "QA Herald"
		b.AccentColorHex = "#12B886"
		b.Prefix = "QHR"
		b.DisplayName = "QA Herald"
		b.DefaultPort = 0
		b.Mission = "QA bot — pherald ↔ Telegram round-trip automation + docs/qa/ evidence"
	default:
		b.AppName = "Herald"
		b.AccentColorHex = "#3D5170"
		b.Prefix = "HER"
		b.DisplayName = "Herald"
		b.DefaultPort = 0
		b.Mission = ""
	}
	b.Flavor = flavor
	b.DefaultFooter = "Sent by " + b.BinaryName + " " + version + " · github.com/vasic-digital/Herald"
	return b
}

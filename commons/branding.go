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
		b.AppName = "Build Herald"
		b.AccentColorHex = "#F6C343"
		b.Prefix = "BHR"
		b.DisplayName = "Build Herald"
		b.DefaultPort = 0
		b.Mission = "CI/build pipeline event fan-out (CLI-only — no serve)"
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
		b.AppName = "Schedule Herald"
		b.AccentColorHex = "#A66DD4"
		b.Prefix = "SCR"
		b.DisplayName = "Schedule Herald"
		b.DefaultPort = 0
		b.Mission = "Cron/scheduled event fan-out (CLI-only — no serve)"
	case "i":
		b.AppName = "Incident Herald"
		b.AccentColorHex = "#E63757"
		b.Prefix = "IHR"
		b.DisplayName = "Incident Herald"
		b.DefaultPort = 24794
		b.Mission = "Incident channel orchestration + status-page webhooks"
	case "r":
		b.AppName = "Release Herald"
		b.AccentColorHex = "#6E84A3"
		b.Prefix = "RHR"
		b.DisplayName = "RHR Herald"
		b.DefaultPort = 0
		b.Mission = "Release-train coordination (CLI-only)"
	case "c":
		b.AppName = "Compliance Herald"
		b.AccentColorHex = "#283E59"
		b.Prefix = "CHR"
		b.DisplayName = "Compliance Herald"
		b.DefaultPort = 24792
		b.Mission = "Compliance event ingestion + audit-trail fan-out"
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

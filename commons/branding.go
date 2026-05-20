// Package commons — Branding factory per spec V3 §6.3 + §11.0.
package commons

// DefaultBranding returns a sensible Branding for the named flavor.
// Each <prefix>herald binary calls this at startup and threads the
// returned Branding through every OutboundMessage; channel adapters
// use it to render rich-message accents.
func DefaultBranding(flavor string, version string) Branding {
	b := Branding{
		BinaryName: flavor + "herald",
	}
	switch flavor {
	case "p":
		b.AppName = "Project Herald"
		b.AccentColorHex = "#2C7BE5"
	case "s":
		b.AppName = "System Herald"
		b.AccentColorHex = "#39AFD1"
	case "b":
		b.AppName = "Build Herald"
		b.AccentColorHex = "#F6C343"
	case "d":
		b.AppName = "Deploy Herald"
		b.AccentColorHex = "#42BA96"
	case "a":
		b.AppName = "Alert Herald"
		b.AccentColorHex = "#DF4759"
	case "sc":
		b.AppName = "Schedule Herald"
		b.AccentColorHex = "#A66DD4"
	case "i":
		b.AppName = "Incident Herald"
		b.AccentColorHex = "#E63757"
	case "r":
		b.AppName = "Release Herald"
		b.AccentColorHex = "#6E84A3"
	case "c":
		b.AppName = "Compliance Herald"
		b.AccentColorHex = "#283E59"
	default:
		b.AppName = "Herald"
		b.AccentColorHex = "#3D5170"
	}
	b.DefaultFooter = "Sent by " + b.BinaryName + " " + version + " · github.com/vasic-digital/Herald"
	return b
}

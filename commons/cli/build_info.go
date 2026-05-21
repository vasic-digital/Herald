package cli

import "runtime"

// buildGoVersion strips the leading "go" from runtime.Version() so
// "go1.25.3" becomes "1.25.3". Tests use this so they can assert
// numeric version shape independently of the runtime prefix.
func buildGoVersion() string {
	v := runtime.Version()
	if len(v) > 2 && v[:2] == "go" {
		return v[2:]
	}
	return v
}

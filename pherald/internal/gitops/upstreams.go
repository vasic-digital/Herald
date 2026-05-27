package gitops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Mirror is one declared upstream mirror parsed from an upstreams/<Host>.sh
// file (the §11.4.36 / Herald multi-host mirror convention). Name is the
// host brand derived from the filename (GitHub / GitLab / GitFlic /
// GitVerse); URL is the UPSTREAMABLE_REPOSITORY value.
type Mirror struct {
	Name string
	URL  string
}

// upstreamURLRE captures the value of an `export UPSTREAMABLE_REPOSITORY=...`
// assignment, tolerating optional surrounding double-quotes and leading
// whitespace. Matches the exact shape of Herald's upstreams/*.sh files.
var upstreamURLRE = regexp.MustCompile(`(?m)^\s*export\s+UPSTREAMABLE_REPOSITORY\s*=\s*"?([^"\n]+?)"?\s*$`)

// ParseUpstreams reads every *.sh under upstreamsDir, extracts the
// UPSTREAMABLE_REPOSITORY value from each, and returns the declared mirrors
// sorted by host name. PURE: it reads files only (never sources/executes the
// scripts — sourcing arbitrary shell would be a §12 host-safety hazard).
//
// A *.sh file without an UPSTREAMABLE_REPOSITORY assignment is skipped (it is
// not a mirror declaration). An empty/absent directory returns (nil, nil) so
// callers can distinguish "zero declared" from a read error.
func ParseUpstreams(upstreamsDir string) ([]Mirror, error) {
	entries, err := os.ReadDir(upstreamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("gitops: read upstreams dir %s: %w", upstreamsDir, err)
	}
	var mirrors []Mirror
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(upstreamsDir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("gitops: read %s: %w", e.Name(), rerr)
		}
		m := upstreamURLRE.FindSubmatch(data)
		if m == nil {
			continue // not a mirror declaration
		}
		mirrors = append(mirrors, Mirror{
			Name: strings.TrimSuffix(e.Name(), ".sh"),
			URL:  strings.TrimSpace(string(m[1])),
		})
	}
	sort.Slice(mirrors, func(i, j int) bool { return mirrors[i].Name < mirrors[j].Name })
	return mirrors, nil
}

// RemoteNameFor returns the lowercase git-remote name a mirror maps to (e.g.
// "github" for the GitHub.sh declaration). "origin" is reserved for the
// primary (GitHub) per Herald convention; callers decide whether to also
// alias the primary as origin.
func (m Mirror) RemoteNameFor() string {
	return strings.ToLower(m.Name)
}

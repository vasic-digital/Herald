package workable

// Item maps one row of the `items` table — the ATMOSphere workable-item
// record shared between ATMOSphere and Herald. The composite primary key
// is (AtmID, CurrentLocation).
type Item struct {
	AtmID           string // e.g. "ATM-238"
	Type            string // 'Bug' | 'Feature' | 'Task' (or "" when a section header)
	Status          string // one of StatusValues
	Severity        string
	Title           string
	Description     string
	ForensicAnchor  string
	ClosureCriteria string
	ComposesWith    string // JSON array, stored as TEXT
	CurrentLocation string // 'Issues' | 'Fixed'
	BodyMd          string
	CreatedAt       string
	LastModified    string
}

// StatusValues is the canonical closed set of 10 workable-item statuses.
// Create/Update reject any status outside this set.
var StatusValues = []string{
	"Queued",
	"In progress",
	"Ready for testing",
	"In testing",
	"Reopened",
	"Operator-blocked",
	"Fixed (→ Fixed.md)",
	"Implemented (→ Fixed.md)",
	"Completed (→ Fixed.md)",
	"Obsolete (→ Fixed.md)",
}

var statusSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(StatusValues))
	for _, v := range StatusValues {
		m[v] = struct{}{}
	}
	return m
}()

// ValidStatus reports whether status is a member of the closed set.
func ValidStatus(status string) bool {
	_, ok := statusSet[status]
	return ok
}

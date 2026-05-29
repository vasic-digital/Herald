package workable

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	// atmIDRe matches the canonical [ATM-NNN] bracket id.
	atmIDRe = regexp.MustCompile(`\[(ATM-\d+)\]`)

	// metaRe matches a metadata line: **Status:** value (Status/Type/Severity).
	metaRe = regexp.MustCompile(`(?i)^\*\*(Status|Type|Severity):\*\*\s*(.+?)\s*$`)
)

// ParseTracker parses ATMOSphere's real Markdown tracker format and
// returns the workable items it finds, each tagged with the supplied
// location ("Issues" or "Fixed").
//
// Items are H2 headings (`## ...`) in any of these shapes:
//
//	## <prefix> — [ATM-NNN] <title>
//	## §<letters>[ optional CRITICAL] — [ATM-NNN] <title>
//	## A. <section title>                 (section header — skipped)
//
// A heading carrying a `[ATM-NNN]` bracket takes that as its id; a
// heading without one gets a stable id derived (sha1) from the heading
// text. A heading whose body contains no `**Status:**` line is treated
// as a section header and skipped. The raw body block under each item
// heading (up to but excluding the next H2) is captured as BodyMd.
func ParseTracker(markdown, location string) ([]Item, error) {
	lines := strings.Split(markdown, "\n")

	type block struct {
		heading string
		body    []string
	}
	var blocks []block
	var cur *block

	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			blocks = append(blocks, block{heading: strings.TrimSpace(ln[3:])})
			cur = &blocks[len(blocks)-1]
			continue
		}
		if cur != nil {
			cur.body = append(cur.body, ln)
		}
	}

	var items []Item
	for _, b := range blocks {
		bodyMd := strings.TrimRight(strings.Join(b.body, "\n"), "\n")
		status, typ, sev := scanMeta(b.body)

		// No status -> section header, not a workable item.
		if status == "" {
			continue
		}

		title := headingTitle(b.heading)
		id := ""
		if m := atmIDRe.FindStringSubmatch(b.heading); m != nil {
			id = m[1]
		} else {
			id = deriveID(b.heading)
		}

		items = append(items, Item{
			AtmID:           id,
			Type:            typ,
			Status:          status,
			Severity:        sev,
			Title:           title,
			CurrentLocation: location,
			BodyMd:          bodyMd,
		})
	}

	return items, nil
}

// scanMeta extracts Status/Type/Severity from a body block's metadata
// lines. Returns empty strings for fields not present.
func scanMeta(body []string) (status, typ, sev string) {
	for _, ln := range body {
		m := metaRe.FindStringSubmatch(strings.TrimSpace(ln))
		if m == nil {
			continue
		}
		switch strings.ToLower(m[1]) {
		case "status":
			status = m[2]
		case "type":
			typ = m[2]
		case "severity":
			sev = m[2]
		}
	}
	return
}

// headingTitle strips the prefix segment and any [ATM-NNN] bracket from
// a heading, returning the human-readable title.
//
//	"§GL CRITICAL — [ATM-238] Netflix login failure on D3" -> "Netflix login failure on D3"
//	"SYS — [ATM-101] Disk pressure alerting"               -> "Disk pressure alerting"
//	"§UX — Tidy the onboarding copy"                        -> "Tidy the onboarding copy"
//	"A. Global blockers"                                    -> "Global blockers"
func headingTitle(heading string) string {
	title := heading

	// Split on the em-dash separator if present; the title is after it.
	if idx := strings.Index(title, " — "); idx >= 0 {
		title = title[idx+len(" — "):]
	} else if idx := strings.Index(title, " - "); idx >= 0 {
		title = title[idx+len(" - "):]
	} else if dot := strings.Index(title, ". "); dot >= 0 && dot <= 3 {
		// "A. Global blockers" style section header.
		title = title[dot+2:]
	}

	// Drop a leading [ATM-NNN] bracket if it survived the split.
	title = atmIDRe.ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

// deriveID produces a stable, deterministic id for a bracket-less
// heading: "ATM-DERIVED-<8hexchars>" of the sha1 of the heading text.
func deriveID(heading string) string {
	sum := sha1.Sum([]byte(heading))
	return "ATM-DERIVED-" + hex.EncodeToString(sum[:])[:8]
}

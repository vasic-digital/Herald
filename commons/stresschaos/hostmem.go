package stresschaos

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// hostMemDarwin reads host memory on macOS via `sysctl hw.memsize` (total)
// and `vm_stat` (page-level breakdown). Best-effort: any failure yields a
// probe-unavailable snapshot. Used-memory is computed as total minus the
// "free + inactive" reclaimable pages, the conventional macOS approximation
// of available memory.
func hostMemDarwin(now string) MemSnapshot {
	snap := MemSnapshot{Platform: "darwin", CapturedAtRFC: now}

	total, err := sysctlUint("hw.memsize")
	if err != nil {
		snap.Note = "probe-unavailable: sysctl hw.memsize: " + err.Error()
		return snap
	}

	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		snap.Note = "probe-unavailable: vm_stat: " + err.Error()
		return snap
	}

	pageSize := uint64(4096)
	var freePages, inactivePages, specPages uint64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		// First line: "Mach Virtual Memory Statistics: (page size of 16384 bytes)"
		if strings.Contains(line, "page size of") {
			if ps := extractPageSize(line); ps > 0 {
				pageSize = ps
			}
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		valStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[colon+1:]), "."))
		val, perr := strconv.ParseUint(valStr, 10, 64)
		if perr != nil {
			continue
		}
		switch key {
		case "Pages free":
			freePages = val
		case "Pages inactive":
			inactivePages = val
		case "Pages speculative":
			specPages = val
		}
	}

	freeBytes := (freePages + inactivePages + specPages) * pageSize
	if freeBytes > total {
		freeBytes = total
	}
	used := total - freeBytes
	snap.Available = true
	snap.TotalBytes = total
	snap.FreeBytes = freeBytes
	snap.UsedBytes = used
	if total > 0 {
		snap.UsedFraction = float64(used) / float64(total)
	}
	return snap
}

// extractPageSize pulls the page size out of the vm_stat header line
// "(page size of 16384 bytes)". Returns 0 if not parseable.
func extractPageSize(line string) uint64 {
	const marker = "page size of "
	i := strings.Index(line, marker)
	if i < 0 {
		return 0
	}
	rest := line[i+len(marker):]
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// sysctlUint runs `sysctl -n <name>` and parses the result as a uint64.
func sysctlUint(name string) (uint64, error) {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	return v, nil
}

// hostMemLinux reads /proc/meminfo (MemTotal + MemAvailable). Best-effort:
// any failure yields a probe-unavailable snapshot.
func hostMemLinux(now string) MemSnapshot {
	snap := MemSnapshot{Platform: "linux", CapturedAtRFC: now}

	f, err := os.Open("/proc/meminfo")
	if err != nil {
		snap.Note = "probe-unavailable: open /proc/meminfo: " + err.Error()
		return snap
	}
	defer f.Close()

	var total, avail uint64
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// fields[1] is in kB; convert to bytes.
		switch fields[0] {
		case "MemTotal:":
			if v, perr := strconv.ParseUint(fields[1], 10, 64); perr == nil {
				total = v * 1024
				haveTotal = true
			}
		case "MemAvailable:":
			if v, perr := strconv.ParseUint(fields[1], 10, 64); perr == nil {
				avail = v * 1024
				haveAvail = true
			}
		}
	}
	if !haveTotal || !haveAvail || total == 0 {
		snap.Note = "probe-unavailable: /proc/meminfo missing MemTotal/MemAvailable"
		return snap
	}
	if avail > total {
		avail = total
	}
	used := total - avail
	snap.Available = true
	snap.TotalBytes = total
	snap.FreeBytes = avail
	snap.UsedBytes = used
	snap.UsedFraction = float64(used) / float64(total)
	return snap
}

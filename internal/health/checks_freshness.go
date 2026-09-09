package health

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Freshness thresholds. Distribution lag is a vulnerability class of its own:
// kernel and runtime fixes land upstream faster than stable distributions
// backport them, so "old but supported" still means known-exploitable windows.
const (
	// kernelBuildMaxAge is how old the running kernel's build may be before
	// the check warns (6 months).
	kernelBuildMaxAge = 180 * 24 * time.Hour
	// distroEOLWarnWindow warns this long BEFORE a release's standard support
	// ends, so the user can plan an upgrade instead of discovering the cliff.
	distroEOLWarnWindow = 180 * 24 * time.Hour
)

// kernelBuildDateRe matches the trailing "Day Mon DD HH:MM:SS ZONE YYYY"
// build timestamp in /proc/version (Ubuntu/Fedora/Arch style).
var kernelBuildDateRe = regexp.MustCompile(
	`[A-Z][a-z]{2} ([A-Z][a-z]{2}) +(\d{1,2}) (\d{2}:\d{2}:\d{2}) (\S+) (\d{4})`)

// kernelBuildDateISORe matches the Debian-style trailing "(YYYY-MM-DD)".
var kernelBuildDateISORe = regexp.MustCompile(`\((\d{4}-\d{2}-\d{2})\)\s*$`)

// parseKernelBuildDate extracts the kernel build date from /proc/version
// content. Zone abbreviations are parsed positionally (offset ignored), which
// is fine at day granularity. Returns false when no date is recognized.
func parseKernelBuildDate(procVersion string) (time.Time, bool) {
	if m := kernelBuildDateRe.FindStringSubmatch(procVersion); m != nil {
		// Re-assemble without the zone so unknown abbreviations don't fail.
		s := fmt.Sprintf("%s %s %s %s", m[1], m[2], m[3], m[5])
		if t, err := time.Parse("Jan 2 15:04:05 2006", s); err == nil {
			return t, true
		}
	}
	if m := kernelBuildDateISORe.FindStringSubmatch(strings.TrimSpace(procVersion)); m != nil {
		if t, err := time.Parse("2006-01-02", m[1]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// evaluateKernelBuildAge is the pure core of CheckKernelBuildAge.
func evaluateKernelBuildAge(procVersion string, now time.Time) HealthCheck {
	built, ok := parseKernelBuildDate(procVersion)
	if !ok {
		// Tolerant like CheckKernelVersionHealth: an unparseable format is
		// not a health problem.
		return HealthCheck{
			Name:    "kernel_build_age",
			Status:  StatusOK,
			Message: "Could not determine kernel build date",
		}
	}
	return evaluateKernelBuildTime(built, now)
}

// evaluateKernelBuildTime turns a known kernel build time into the check result.
func evaluateKernelBuildTime(built, now time.Time) HealthCheck {
	age := now.Sub(built)
	days := int(age.Hours() / 24)
	details := map[string]interface{}{
		"built_at": built.Format("2006-01-02"),
		"age_days": days,
	}
	if age > kernelBuildMaxAge {
		return HealthCheck{
			Name:   "kernel_build_age",
			Status: StatusWarning,
			Message: fmt.Sprintf("Running kernel was built %d days ago — security patches land upstream faster than distributions backport them; consider updating the host kernel",
				days),
			Details: details,
		}
	}
	return HealthCheck{
		Name:    "kernel_build_age",
		Status:  StatusOK,
		Message: fmt.Sprintf("Kernel built %d days ago", days),
		Details: details,
	}
}

// CheckKernelBuildAge warns when the running kernel's build date is older than
// kernelBuildMaxAge. The kernel is the shared isolation boundary for every COI
// container, so a stale build means months of unpatched kernel fixes.
func CheckKernelBuildAge() HealthCheck {
	if runtime.GOOS != "linux" {
		return HealthCheck{
			Name:    "kernel_build_age",
			Status:  StatusOK,
			Message: fmt.Sprintf("Not applicable (%s)", runtime.GOOS),
		}
	}
	content, err := os.ReadFile("/proc/version")
	if err != nil {
		return HealthCheck{
			Name:    "kernel_build_age",
			Status:  StatusOK,
			Message: "Could not read /proc/version",
		}
	}
	check := evaluateKernelBuildAge(string(content), time.Now())
	if _, parsed := parseKernelBuildDate(string(content)); !parsed {
		// The UTS version string is capped at 64 bytes, so kernels with long
		// banners (e.g. Ubuntu HWE "#30~24.04.1-Ubuntu SMP ...") lose the
		// trailing year. Fall back to the installed modules directory's
		// mtime, which tracks when this kernel build landed on the host.
		if release, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			modulesDir := "/lib/modules/" + strings.TrimSpace(string(release))
			if fi, err := os.Stat(modulesDir); err == nil { //nolint:gosec // G703: path component is the kernel's own release string, not user input
				return evaluateKernelBuildTime(fi.ModTime(), time.Now())
			}
		}
	}
	return check
}

// distroEOL is the end of STANDARD security support (not paid/extended
// support) for the distributions COI commonly runs on. Small and static on
// purpose: unknown entries simply return OK, and the dates only need to be
// right to the month.
var distroEOL = map[string]map[string]time.Time{
	"ubuntu": {
		"20.04": time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC),
		"22.04": time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
		"24.04": time.Date(2029, 5, 31, 0, 0, 0, 0, time.UTC),
		"26.04": time.Date(2031, 5, 31, 0, 0, 0, 0, time.UTC),
	},
	"debian": {
		"11": time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		"12": time.Date(2028, 6, 30, 0, 0, 0, 0, time.UTC),
		"13": time.Date(2030, 6, 30, 0, 0, 0, 0, time.UTC),
	},
}

// parseOSRelease extracts ID and VERSION_ID from /etc/os-release content.
func parseOSRelease(content string) (id, versionID string) {
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		case strings.HasPrefix(line, "VERSION_ID="):
			versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	return id, versionID
}

// evaluateDistroEOL is the pure core of CheckDistroEOL.
func evaluateDistroEOL(osRelease string, now time.Time) HealthCheck {
	id, versionID := parseOSRelease(osRelease)
	if id == "" || versionID == "" {
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: "Could not determine distribution release",
		}
	}
	versions, ok := distroEOL[id]
	if !ok {
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: fmt.Sprintf("%s %s (no EOL data)", id, versionID),
		}
	}
	eol, ok := versions[versionID]
	if !ok {
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: fmt.Sprintf("%s %s (no EOL data for this release)", id, versionID),
		}
	}

	details := map[string]interface{}{
		"distro":  id,
		"version": versionID,
		"eol":     eol.Format("2006-01-02"),
	}
	// Note when a newer release of the same distro is in the table: running
	// "oldstable" means backports arrive later than for the current release.
	for v, otherEOL := range versions {
		if otherEOL.After(eol) {
			details["newer_release"] = v
			break
		}
	}

	switch {
	case now.After(eol):
		return HealthCheck{
			Name:   "distro_eol",
			Status: StatusWarning,
			Message: fmt.Sprintf("%s %s reached end of standard support on %s — security backports have stopped; upgrade the host",
				id, versionID, eol.Format("2006-01-02")),
			Details: details,
		}
	case now.Add(distroEOLWarnWindow).After(eol):
		return HealthCheck{
			Name:   "distro_eol",
			Status: StatusWarning,
			Message: fmt.Sprintf("%s %s standard support ends %s — plan a host upgrade",
				id, versionID, eol.Format("2006-01-02")),
			Details: details,
		}
	default:
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: fmt.Sprintf("%s %s supported until %s", id, versionID, eol.Format("2006-01-02")),
			Details: details,
		}
	}
}

// CheckDistroEOL warns when the host distribution is past (or within six
// months of) the end of standard security support. An EOL host means the
// shared kernel and system libraries under every container stop receiving
// fixes entirely.
func CheckDistroEOL() HealthCheck {
	if runtime.GOOS != "linux" {
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: fmt.Sprintf("Not applicable (%s)", runtime.GOOS),
		}
	}
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return HealthCheck{
			Name:    "distro_eol",
			Status:  StatusOK,
			Message: "Could not read /etc/os-release",
		}
	}
	return evaluateDistroEOL(string(content), time.Now())
}

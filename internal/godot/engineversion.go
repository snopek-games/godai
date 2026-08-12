package godot

import (
	"fmt"
	"strconv"
	"strings"
)

// EngineVersion identifies one build of the Godot engine, the way the
// godot-builds releases do: a version number, a release status, and whether
// it's the .NET ("mono") build.
type EngineVersion struct {
	Major  int
	Minor  int
	Patch  int
	Status string
	Mono   bool
}

const StatusStable = "stable"

// VersionError says why something the user typed isn't a Godot version, in a
// sentence that can be shown as-is.
type VersionError struct {
	Input  string
	Reason string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("%q isn't a Godot version: %s", e.Input, e.Reason)
}

// statusRanks orders the release statuses Godot publishes. A status that isn't
// listed sorts before all of them.
var statusRanks = map[string]int{
	"dev":    1,
	"alpha":  2,
	"beta":   3,
	"rc":     4,
	"stable": 5,
}

// ParseEngineVersion reads a version the user typed, such as "4.5", "4.4.1",
// "4.6-beta3" or "4.5-stable-mono". A version with no status is stable.
func ParseEngineVersion(s string) (EngineVersion, error) {
	original := s

	reject := func(reason string, args ...any) (EngineVersion, error) {
		return EngineVersion{}, &VersionError{Input: original, Reason: fmt.Sprintf(reason, args...)}
	}

	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return reject("there's nothing there")
	}

	version := EngineVersion{Status: StatusStable}
	if rest, ok := strings.CutSuffix(s, "-mono"); ok {
		version.Mono = true
		s = rest
	}

	numbers := s
	if before, after, ok := strings.Cut(s, "-"); ok {
		numbers = before
		version.Status = after
	}

	if reason := statusProblem(version.Status); reason != "" {
		return reject("%s", reason)
	}

	parts := strings.Split(numbers, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return reject("expected a number like 4.5 or 4.4.1")
	}

	values := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return reject("%q isn't a number", part)
		}
		values[i] = n
	}

	version.Major, version.Minor, version.Patch = values[0], values[1], values[2]
	return version, nil
}

// ParseVersionOutput reads what `godot --version` prints, which looks like
// "4.5.stable.official.a2b3c4d5e", or "4.4.1.stable.mono.official.b09f793f5"
// for a .NET build. Other lines are skipped, since a build can print warnings
// before it gets to answering.
func ParseVersionOutput(out string) (EngineVersion, error) {
	for line := range strings.SplitSeq(out, "\n") {
		if version, err := parseVersionLine(line); err == nil {
			return version, nil
		}
	}

	return EngineVersion{}, &VersionError{
		Input:  strings.TrimSpace(out),
		Reason: "expected something like 4.5.stable.official.a2b3c4d5e",
	}
}

func parseVersionLine(line string) (EngineVersion, error) {
	fields := strings.Split(strings.ToLower(strings.TrimSpace(line)), ".")

	numbers := 0
	for numbers < len(fields) && isNumber(fields[numbers]) {
		numbers++
	}
	if numbers < 2 || numbers > 3 || numbers == len(fields) {
		return EngineVersion{}, &VersionError{Input: line, Reason: "not a version Godot prints"}
	}

	version := strings.Join(fields[:numbers], ".") + "-" + fields[numbers]
	if numbers+1 < len(fields) && fields[numbers+1] == "mono" {
		version += "-mono"
	}

	return ParseEngineVersion(version)
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
}

func statusProblem(status string) string {
	name := strings.TrimRight(status, "0123456789")
	number := status[len(name):]

	switch {
	case name == "":
		return fmt.Sprintf("%q isn't a release status", status)
	case !isKnownStatus(name):
		return fmt.Sprintf("unknown release status %q; expected dev, alpha, beta, rc or stable", name)
	case name == StatusStable && number != "":
		return fmt.Sprintf("%q isn't a release status", status)
	}

	return ""
}

func isKnownStatus(name string) bool {
	_, known := statusRanks[name]
	return known
}

// Number is just the number part of the version on its own, ex "4.5" or "4.4.1".
func (v EngineVersion) Number() string {
	if v.Patch == 0 {
		return fmt.Sprintf("%d.%d", v.Major, v.Minor)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Tag is the release tag on godotengine/godot-builds (doesn't include .NET marker).
func (v EngineVersion) Tag() string {
	return v.Number() + "-" + v.Status
}

func (v EngineVersion) String() string {
	if v.Mono {
		return v.Tag() + "-mono"
	}
	return v.Tag()
}

// TemplateDirName is what Godot calls this version's directory under export_templates.
func (v EngineVersion) TemplateDirName() string {
	name := v.Number() + "." + v.Status
	if v.Mono {
		name += ".mono"
	}
	return name
}

func (v EngineVersion) IsStable() bool {
	return v.Status == StatusStable
}

// Separates a real Godot version from a linked engine version.
func (v EngineVersion) Known() bool {
	return v.Major > 0
}

func (v EngineVersion) Compare(other EngineVersion) int {
	for _, pair := range [][2]int{
		{v.Major, other.Major},
		{v.Minor, other.Minor},
		{v.Patch, other.Patch},
	} {
		if c := compareInt(pair[0], pair[1]); c != 0 {
			return c
		}
	}

	rank, number := splitStatus(v.Status)
	otherRank, otherNumber := splitStatus(other.Status)
	if c := compareInt(rank, otherRank); c != 0 {
		return c
	}
	if c := compareInt(number, otherNumber); c != 0 {
		return c
	}

	return compareInt(boolToInt(v.Mono), boolToInt(other.Mono))
}

func splitStatus(status string) (rank, number int) {
	name := strings.TrimRight(status, "0123456789")
	number, _ = strconv.Atoi(status[len(name):])
	return statusRanks[name], number
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

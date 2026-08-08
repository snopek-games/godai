package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Prerelease          string
}

func ParseVersion(s string) (Version, error) {
	original := s

	s = strings.TrimPrefix(s, "v")

	// Build metadata takes no part in precedence.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	var prerelease string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		prerelease = s[i+1:]
		s = s[:i]
		if prerelease == "" {
			return Version{}, fmt.Errorf("%q has an empty pre-release suffix", original)
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%q isn't a version like 1.2.3", original)
	}

	numbers := make([]int, len(parts))
	for i, part := range parts {
		if part == "" || strings.ContainsFunc(part, func(r rune) bool { return r < '0' || r > '9' }) {
			return Version{}, fmt.Errorf("%q isn't a version like 1.2.3", original)
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("%q isn't a version like 1.2.3: %w", original, err)
		}
		numbers[i] = n
	}

	return Version{
		Major:      numbers[0],
		Minor:      numbers[1],
		Patch:      numbers[2],
		Prerelease: prerelease,
	}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	return s
}

func (v Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

// Compare returns -1 if v sorts before other, 1 if it sorts after, and 0 if
// they're equal, following the semantic versioning precedence rules.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{
		{v.Major, other.Major},
		{v.Minor, other.Minor},
		{v.Patch, other.Patch},
	} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}

	switch {
	case v.Prerelease == "" && other.Prerelease == "":
		return 0
	case v.Prerelease == "":
		return 1
	case other.Prerelease == "":
		return -1
	}

	return comparePrerelease(v.Prerelease, other.Prerelease)
}

// comparePrerelease compares two pre-release suffixes identifier by identifier:
// numeric identifiers sort numerically and before non-numeric ones, which sort
// by ASCII. When one suffix is a prefix of the other, the shorter one is lower.
func comparePrerelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		aNum, aIsNum := parseIdentifier(aParts[i])
		bNum, bIsNum := parseIdentifier(bParts[i])

		switch {
		case aIsNum && bIsNum:
			if aNum != bNum {
				if aNum < bNum {
					return -1
				}
				return 1
			}
		case aIsNum:
			return -1
		case bIsNum:
			return 1
		default:
			if c := strings.Compare(aParts[i], bParts[i]); c != 0 {
				return c
			}
		}
	}

	switch {
	case len(aParts) < len(bParts):
		return -1
	case len(aParts) > len(bParts):
		return 1
	}
	return 0
}

func parseIdentifier(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

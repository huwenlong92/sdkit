package baidupan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`(?i)\bv?(\d+)\.(\d+)\.(\d+)\b`)

type version struct {
	major int
	minor int
	patch int
}

func parseVersion(value string) (version, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 4 {
		return version{}, fmt.Errorf("invalid semantic version")
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return version{}, err
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return version{}, err
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return version{}, err
	}
	return version{major: major, minor: minor, patch: patch}, nil
}

func (v version) compare(other version) int {
	if v.major != other.major {
		return compareInt(v.major, other.major)
	}
	if v.minor != other.minor {
		return compareInt(v.minor, other.minor)
	}
	return compareInt(v.patch, other.patch)
}

func compareInt(left int, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareVersionStrings(left string, right string) int {
	leftVersion, _ := parseVersion(left)
	rightVersion, _ := parseVersion(right)
	return leftVersion.compare(rightVersion)
}

func normalizeVersion(value string) (string, error) {
	parsed, err := parseVersion(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("v%d.%d.%d", parsed.major, parsed.minor, parsed.patch), nil
}

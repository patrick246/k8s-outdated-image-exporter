package version

import (
	"github.com/hashicorp/go-version"
	"sort"
	"strings"
)

type Checker struct {
}

func NewChecker() (*Checker, error) {
	return &Checker{}, nil
}

func (c *Checker) GetDifference(current string, available []string) (major, minor, patch int64, err error) {
	currentParsed, err := version.NewSemver(current)
	if err != nil {
		return
	}

	versions := make([]*version.Version, 0, len(available))
	for _, v := range available {
		// If it doesn't start with a v and doesn't contain a dot, then it's most likely not a semver
		if !strings.HasPrefix(v, "v") && !strings.Contains(v, ".") {
			continue
		}

		parsedVersion, err := version.NewSemver(v)
		// Skip non-semver versions
		if err != nil {
			continue
		}
		// Skip prereleases
		if parsedVersion.Prerelease() != "" {
			continue
		}
		// Skip all older version
		if parsedVersion.LessThanOrEqual(currentParsed) {
			continue
		}

		versions = append(versions, parsedVersion)
	}

	if len(versions) == 0 {
		return
	}

	sort.Sort(version.Collection(versions))

	latestVersion := versions[len(versions)-1]
	latestSegments := latestVersion.Segments64()
	currentSegments := currentParsed.Segments64()
	if latestSegments[0] > currentSegments[0] {
		major = latestSegments[0] - currentSegments[0]
		return
	}
	if latestSegments[1] > currentSegments[1] {
		minor = latestSegments[1] - currentSegments[1]
		return
	}
	if latestSegments[2] > currentSegments[2] {
		patch = latestSegments[2] - currentSegments[2]
		return
	}
	return
}

package version_test

import (
	"fmt"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/version"
	"github.com/stretchr/testify/require"
	"testing"
)

type testcase struct {
	Current     string
	Available   []string
	PinMode     version.PinMode
	ResultMajor int64
	ResultMinor int64
	ResultPatch int64
}

func TestChecker_GetDifference(t *testing.T) {
	versionChecker, err := version.NewChecker()
	require.NoError(t, err)

	testCases := []testcase{{
		Current:     "v1.0.0",
		Available:   []string{"v0.9.8", "v0.9.9", "1.0.0", "1.0.1", "1.1.0"},
		PinMode:     version.PIN_NONE,
		ResultMajor: 0,
		ResultMinor: 1,
		ResultPatch: 0,
	}, {
		Current:     "v2.0.0",
		Available:   []string{"1.0.0", "1.2.0", "2.0.0"},
		PinMode:     version.PIN_NONE,
		ResultMajor: 0,
		ResultMinor: 0,
		ResultPatch: 0,
	}, {
		Current:     "v1.0.0",
		Available:   []string{"1.0.0", "2.0.0", "3.0.0"},
		PinMode:     version.PIN_NONE,
		ResultMajor: 2,
		ResultMinor: 0,
		ResultPatch: 0,
	}, {
		Current:     "1.0",
		Available:   []string{"1", "1.0", "1.0.1"},
		PinMode:     version.PIN_NONE,
		ResultMajor: 0,
		ResultMinor: 0,
		ResultPatch: 1,
	}, {
		Current:     "v1.0.0",
		Available:   []string{"v0.9.8", "v0.9.9", "1.0.0", "1.0.1", "1.1.0", "1.2.0", "2.0.0", "2.0.1"},
		PinMode:     version.PIN_MAJOR,
		ResultMajor: 0,
		ResultMinor: 2,
		ResultPatch: 0,
	}, {
		Current:     "v1.0.0",
		Available:   []string{"v0.9.8", "v0.9.9", "1.0.0", "1.0.1", "1.1.0", "1.2.0", "2.0.0", "2.0.1"},
		PinMode:     version.PIN_MINOR,
		ResultMajor: 0,
		ResultMinor: 0,
		ResultPatch: 1,
	}}

	for _, testcase := range testCases {
		t.Run(fmt.Sprintf("current=%s;available=%s;pinMode=%d", testcase.Current, testcase.Available, testcase.PinMode), func(t *testing.T) {
			major, minor, patch, err := versionChecker.GetDifference(testcase.Current, testcase.Available, testcase.PinMode)
			require.NoError(t, err)

			require.Equal(t, testcase.ResultMajor, major)
			require.Equal(t, testcase.ResultMinor, minor)
			require.Equal(t, testcase.ResultPatch, patch)
		})
	}
}

package mods

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_encodeFilename(t *testing.T) {
	version := "v1.0.0"
	projectName := "dummy/project"

	filename := encodeFilename(projectName, version)

	decodedVersion, err := decodeFilename(filename)

	require.NoError(t, err)
	require.Equal(t, version, decodedVersion)
}

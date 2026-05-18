package ui

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssets_ContainsIndexHTML(t *testing.T) {
	t.Skip("requires `make ui-build && cp -r ui/dist internal/ui/dist` before running")

	assets, err := Assets()
	require.NoError(t, err)

	f, err := assets.Open("index.html")
	require.NoError(t, err)
	defer f.Close()

	info, err := f.(interface{ Stat() (fs.FileInfo, error) }).Stat()
	require.NoError(t, err)
	require.False(t, info.IsDir())
}

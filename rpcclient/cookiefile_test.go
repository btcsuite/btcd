package rpcclient

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadCookieFile ensures cookie credentials are parsed from the first line
// without altering valid password contents.
func TestReadCookieFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contents     string
		wantUsername string
		wantPassword string
	}{
		{
			name:         "standard credentials",
			contents:     "__cookie__:secret\n",
			wantUsername: "__cookie__",
			wantPassword: "secret",
		},
		{
			name:         "password containing colons",
			contents:     "__cookie__:secret:with:colons\n",
			wantUsername: "__cookie__",
			wantPassword: "secret:with:colons",
		},
		{
			name:         "CRLF line ending",
			contents:     "__cookie__:secret\r\n",
			wantUsername: "__cookie__",
			wantPassword: "secret",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cookiePath := filepath.Join(t.TempDir(), ".cookie")
			err := os.WriteFile(
				cookiePath, []byte(test.contents), 0o600,
			)
			require.NoError(t, err)

			username, password, err := readCookieFile(cookiePath)
			require.NoError(t, err)
			require.Equal(t, test.wantUsername, username)
			require.Equal(t, test.wantPassword, password)
		})
	}
}

// TestReadCookieFileErrors ensures invalid cookie files return an error and no
// partial credentials.
func TestReadCookieFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{
			name:     "empty file",
			contents: "",
		},
		{
			name:     "missing separator",
			contents: "__cookie__\n",
		},
		{
			name: "scanner token too long",
			contents: strings.Repeat(
				"a", bufio.MaxScanTokenSize+1,
			),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cookiePath := filepath.Join(t.TempDir(), ".cookie")
			err := os.WriteFile(
				cookiePath, []byte(test.contents), 0o600,
			)
			require.NoError(t, err)

			username, password, err := readCookieFile(cookiePath)
			require.Error(t, err)
			require.Empty(t, username)
			require.Empty(t, password)
		})
	}

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		cookiePath := filepath.Join(t.TempDir(), "missing.cookie")
		username, password, err := readCookieFile(cookiePath)
		require.Error(t, err)
		require.Empty(t, username)
		require.Empty(t, password)
	})
}

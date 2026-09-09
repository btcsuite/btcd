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
// without altering valid password contents, and invalid cookie files return an
// error without partial credentials.
func TestReadCookieFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filename     string
		contents     string
		wantUsername string
		wantPassword string
		expectError  bool
	}{
		{
			name:         "standard credentials",
			filename:     ".cookie",
			contents:     "__cookie__:secret\n",
			wantUsername: "__cookie__",
			wantPassword: "secret",
		},
		{
			name:         "password containing colons",
			filename:     ".cookie",
			contents:     "__cookie__:secret:with:colons\n",
			wantUsername: "__cookie__",
			wantPassword: "secret:with:colons",
		},
		{
			name:         "CRLF line ending",
			filename:     ".cookie",
			contents:     "__cookie__:secret\r\n",
			wantUsername: "__cookie__",
			wantPassword: "secret",
		},
		{
			name:        "empty file",
			filename:    ".cookie",
			expectError: true,
		},
		{
			name:        "missing separator",
			filename:    ".cookie",
			contents:    "__cookie__\n",
			expectError: true,
		},
		{
			name:     "scanner token too long",
			filename: ".cookie",
			contents: strings.Repeat(
				"a", bufio.MaxScanTokenSize+1,
			),
			expectError: true,
		},
		{
			name:        "missing file",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cookiePath := filepath.Join(dir, "missing.cookie")
			if test.filename != "" {
				cookiePath = filepath.Join(dir, test.filename)
				err := os.WriteFile(
					cookiePath,
					[]byte(test.contents),
					0o600,
				)
				require.NoError(t, err)
			}

			username, password, err := readCookieFile(cookiePath)
			if test.expectError {
				require.Error(t, err)
				require.Empty(t, username)
				require.Empty(t, password)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantUsername, username)
			require.Equal(t, test.wantPassword, password)
		})
	}
}

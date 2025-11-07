package fsutil

import (
	"os"
	"path"
)

// the name of an environment variable at runtime
const TEST_TMPDIR = "TEST_TMPDIR"

// NewTmpDir creates a new temporary directory in TestTmpDir().
func NewTmpDir(prefix string) (string, error) {
	if tmp, ok := os.LookupEnv(TEST_TMPDIR); ok {
		err := os.MkdirAll(path.Join(tmp, prefix), 0700)
		return tmp, err
	}
	return os.MkdirTemp("", prefix)
}

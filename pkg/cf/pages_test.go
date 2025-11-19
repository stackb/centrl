package cf

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func TestReadTarball(t *testing.T) {
	// Create a test tarball in memory
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files := map[string]string{
		"index.html": "<html></html>",
		"app.js":     "console.log('test');",
		"styles.css": "body { margin: 0; }",
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Override openFile to use our in-memory tarball
	oldOpenFile := openFile
	defer func() { openFile = oldOpenFile }()

	openFile = func(path string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}

	// Test readTarball
	result, err := readTarball("test.tar")
	if err != nil {
		t.Fatalf("readTarball failed: %v", err)
	}

	if len(result) != len(files) {
		t.Errorf("expected %d files, got %d", len(files), len(result))
	}

	for name, expectedContent := range files {
		actualContent, ok := result[name]
		if !ok {
			t.Errorf("file %s not found in result", name)
			continue
		}
		if string(actualContent) != expectedContent {
			t.Errorf("file %s: expected %q, got %q", name, expectedContent, string(actualContent))
		}
	}
}
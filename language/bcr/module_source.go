package bcr

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/sourcejson"
)

func moduleSourceLoadInfo() rule.LoadInfo {
	return rule.LoadInfo{
		Name:    "//rules:module_source.bzl",
		Symbols: []string{"module_source"},
	}
}

func moduleSourceKinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"module_source": {
			MatchAny:     true,
			ResolveAttrs: map[string]bool{"docs_url": true},
		},
	}
}
func readModuleSourceJson(filename string) (*bzpb.ModuleSource, error) {
	return sourcejson.ReadFile(filename)
}

func makeModuleSourceRule(source *bzpb.ModuleSource, sourceJsonFile string, ext *bcrExtension) *rule.Rule {
	r := rule.NewRule("module_source", "source")
	if source.Url != "" {
		r.SetAttr("url", source.Url)
	}
	if source.DocsUrl != "" {
		r.SetAttr("docs_url", source.DocsUrl)
		docsLabel := ext.trackDocsUrl(source.DocsUrl)
		if docsLabel != label.NoLabel {
			r.SetAttr("docs", []string{docsLabel.String()})
		}
	}
	if source.Integrity != "" {
		r.SetAttr("integrity", source.Integrity)
	}
	if source.StripPrefix != "" {
		r.SetAttr("strip_prefix", source.StripPrefix)
	}
	if source.PatchStrip != 0 {
		r.SetAttr("patch_strip", int(source.PatchStrip))
	}
	if len(source.Patches) > 0 {
		r.SetAttr("patches", source.Patches)
	}
	if sourceJsonFile != "" {
		r.SetAttr("source_json", sourceJsonFile)
	}
	return r
}

// resolveModuleSourceRule resolves the docs URL, if present.
func resolveModuleSourceRule(r *rule.Rule, c *config.Config, from label.Label) {
	// docsUrl := r.AttrString("docs_url")
	// if docsUrl != "" {
	// 	if err := resolveModuleSourceDocsUrl(r, c, from, docsUrl); err != nil {
	// 		log.Printf("warn: failed to gather docs from %s: %v", docsUrl, err)
	// 	}
	// }
}

// resolveModuleSourceRule resolves the docs URL, if present.
func resolveModuleSourceDocsUrl(r *rule.Rule, c *config.Config, from label.Label, docsUrl string) error {
	log.Println("resolve docs:", docsUrl, strings.ToLower(filepath.Ext(docsUrl)))

	docsDir := filepath.Join(c.RepoRoot, from.Pkg, "docs")

	if strings.HasSuffix(docsUrl, ".docs.tar.gz") {
		log.Printf("fetching %s => %s", docsUrl, docsDir)
		if err := downloadTarGzToDir(docsUrl, docsDir); err != nil {
			return err
		}
		r.SetAttr("docs", []string{"docs/**/*"})
	}

	return nil
}

// downloadTarGzToDir will download a url and store it in local filepath.
// It writes to the destination file as it downloads it, without
// loading the entire file into memory.  Sha256 is checked before.
func downloadTarGzToDir(srcUrl, dstDir string) error {
	// Get the data
	resp, err := http.Get(srcUrl)
	if err != nil {
		return fmt.Errorf("GET failed %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET: expected 200, got %d", resp.StatusCode)
	}

	var in io.Reader

	in = resp.Body

	if filepath.Ext(srcUrl) == ".gz" {
		r, err := gzip.NewReader(in)
		if err != nil {
			return fmt.Errorf("gunzip reader: %v", err)

		}
		in = r
	}

	return untar(in, dstDir, "")
}

// untar takes a destination path and a reader; a tar reader loops over the
// tarfile creating the file structure at 'dst' along the way, and writing any
// files
func untar(in io.Reader, dstDir, stripPrefix string) error {
	if stripPrefix != "" && !strings.HasSuffix(stripPrefix, "/") {
		stripPrefix = stripPrefix + "/"
	}

	tr := tar.NewReader(in)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		name := header.Name
		if stripPrefix != "" && strings.HasPrefix(name, stripPrefix) {
			name = name[len(stripPrefix):]
		}

		target := filepath.Join(dstDir, name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer f.Close()

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
}

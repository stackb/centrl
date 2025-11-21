package bcr

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/stackb/centrl/pkg/modulebazel"
)

// trackDocsUrl adds a doc URL to be included in the root MODULE.bazel file and
// returns the label by which is can be included
func (ext *bcrExtension) trackDocsUrl(docUrl string) label.Label {
	if strings.Contains(docUrl, "{OWNER}") || strings.Contains(docUrl, "{REPO}") || strings.Contains(docUrl, "{TAG}") {
		return label.NoLabel
	}

	from := makeDocsLabel(docUrl)
	httpArchive := makeDocsHttpArchive(from, docUrl)
	ext.docs[from] = httpArchive

	return from
}

// mergeModuleBazelFile updates the MODULE.bazel file with additional rules if
// needed.
func (ext *bcrExtension) mergeModuleBazelFile(repoRoot string) error {
	if len(ext.docs) == 0 {
		return nil
	}

	filename := filepath.Join(repoRoot, "MODULE.bazel")
	f, err := modulebazel.LoadFile(filename, "")
	if err != nil {
		return fmt.Errorf("parsing: %v", err)
	}

	// clean old rules
	for _, r := range f.Rules {
		if r.Kind() == "http_archive" && strings.HasSuffix(r.AttrString("url"), ".docs.tar.gz") {
			r.Delete()
		}
	}
	f.Sync()

	for _, r := range ext.docs {
		r.Insert(f)
	}
	f.Sync()

	data := f.Format()
	os.WriteFile(filename, data, 0744)

	log.Println("Updated:", filename)
	return nil
}

// makeDocsLabel creates a label for external workspace
func makeDocsLabel(docUrl string) label.Label {
	return label.New(makeDocsWorkspaceName(docUrl), "", "files")
}

// makeDocsWorkspaceName creates a named for the external workspace
func makeDocsWorkspaceName(docUrl string) (name string) {
	name = strings.TrimSuffix(docUrl, ".docs.tar.gz")
	name = strings.TrimPrefix(name, "https://")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}

func makeDocsHttpArchive(from label.Label, docUrl string) *rule.Rule {
	r := rule.NewRule("http_archive", from.Repo)
	r.SetAttr("url", docUrl)
	r.SetAttr("build_file_content", fmt.Sprintf(`filegroup(name = "%s",
    srcs = glob(["**/*.binaryproto"]),
    visibility = ["//visibility:public"],
)`, from.Name))
	return r
}

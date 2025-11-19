package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/attestationsjson"
	"github.com/stackb/centrl/pkg/modulebazel"
	"github.com/stackb/centrl/pkg/paramsfile"
	"github.com/stackb/centrl/pkg/presubmityml"
	"github.com/stackb/centrl/pkg/protoutil"
	"github.com/stackb/centrl/pkg/sourcejson"
)

const toolName = "moduleversioncompiler"

type Config struct {
	OutputFile           string
	ModuleBazelFile      string
	SourceJsonFile       string
	AttestationsJsonFile string
	PresubmitYmlFile     string
	CommitSha1           string
	CommitDate           string
	CommitMessage        string
	UnresolvedDeps       string
}

func main() {
	log.SetPrefix(toolName + ": ")
	log.SetOutput(os.Stderr)
	log.SetFlags(0) // don't print timestamps

	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	parsedArgs, err := paramsfile.ReadArgsParamsFile(args)
	if err != nil {
		return fmt.Errorf("failed to read params file: %v", err)
	}

	cfg, err := parseFlags(parsedArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}

	// Validate required flags
	if cfg.ModuleBazelFile == "" {
		return fmt.Errorf("module_bazel_file is required")
	}
	if cfg.OutputFile == "" {
		return fmt.Errorf("output_file is required")
	}

	// Read MODULE.bazel file (required)
	module, err := modulebazel.ExecFile(cfg.ModuleBazelFile)
	if err != nil {
		return fmt.Errorf("failed to read MODULE.bazel: %v", err)
	}

	// Read source.json file (optional)
	if cfg.SourceJsonFile != "" {
		source, err := sourcejson.ReadFile(cfg.SourceJsonFile)
		if err != nil {
			return fmt.Errorf("failed to read source.json: %v", err)
		}
		module.Source = source
	}

	// Read presubmit.yml file (optional)
	if cfg.PresubmitYmlFile != "" {
		presubmit, err := presubmityml.ReadFile(cfg.PresubmitYmlFile)
		if err != nil {
			// TODO: fix parsing of the YAML
			log.Printf("failed to read presubmit.yml: %v", err)
		} else {
			module.Presubmit = presubmit
		}
	}

	// Read attestions.json file (optional)
	if cfg.AttestationsJsonFile != "" {
		attestations, err := attestationsjson.ReadFile(cfg.AttestationsJsonFile)
		if err != nil {
			return fmt.Errorf("failed to read presubmit.yml: %v", err)
		}
		module.Attestations = attestations
	}

	// Add commit information (optional)
	if cfg.CommitSha1 != "" && cfg.CommitDate != "" {
		module.Commit = &bzpb.ModuleCommit{
			Sha1:        cfg.CommitSha1,
			Date:        cfg.CommitDate,
			Message:     cfg.CommitMessage,
			PullRequest: parsePullRequestFromCommitMessage(cfg.CommitMessage),
		}
	}

	if cfg.UnresolvedDeps != "" {
		depNames := strings.Split(cfg.UnresolvedDeps, ",")
		for _, name := range depNames {
			dep := mustFindDependencyByName(module, name)
			dep.Unresolved = true
		}
	}

	// Write the compiled ModuleVersion to output file
	if err := protoutil.WriteFile(cfg.OutputFile, module); err != nil {
		return fmt.Errorf("failed to write output file: %v", err)
	}

	// log.Printf("Successfully compiled module version to %s", cfg.OutputFile)
	return nil
}

func parseFlags(args []string) (cfg Config, err error) {
	fs := flag.NewFlagSet(toolName, flag.ExitOnError)
	fs.StringVar(&cfg.ModuleBazelFile, "module_bazel_file", "", "the MODULE.bazel file to read (required)")
	fs.StringVar(&cfg.SourceJsonFile, "source_json_file", "", "the source.json file to read (optional)")
	fs.StringVar(&cfg.PresubmitYmlFile, "presubmit_yml_file", "", "the presubmit.yml file to read (optional)")
	fs.StringVar(&cfg.AttestationsJsonFile, "attestations_json_file", "", "the attestations.json file to read (optional)")
	fs.StringVar(&cfg.CommitSha1, "commit_sha1", "", "the git commit SHA-1 hash (optional)")
	fs.StringVar(&cfg.CommitDate, "commit_date", "", "the git commit date in ISO 8601 format (optional)")
	fs.StringVar(&cfg.CommitMessage, "commit_message", "", "the git commit message (optional)")
	fs.StringVar(&cfg.OutputFile, "output_file", "", "the output file to write")
	fs.StringVar(&cfg.UnresolvedDeps, "unresolved_deps", "", "comma-separated list of dep names that failed to resolve to a known version")

	fs.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s @PARAMS_FILE", toolName)
		fs.PrintDefaults()
	}

	if err = fs.Parse(args); err != nil {
		return
	}

	return
}

// listFiles is a convenience debugging function to log the files under a given dir.
func listFiles(dir string) error {
	log.Println("Listing files under " + dir)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("%v\n", err)
			return err
		}
		log.Println(path)
		return nil
	})
}

func mustFindDependencyByName(module *bzpb.ModuleVersion, name string) *bzpb.ModuleDependency {
	for _, dep := range module.Deps {
		if name == dep.Name {
			return dep
		}
	}
	log.Fatalf("unable to find dependency %s in module version %s/%s (this is a bug)", name, module.Name, module.Version)
	return nil
}

var pullRequestRegex = regexp.MustCompile(`\(#(\d+)\)`)

// parsePullRequestFromCommitMessage extracts the pull request number from a commit message.
// Example: "Add basic support for Boost 1.89.0 (#5514)" returns "5514"
func parsePullRequestFromCommitMessage(message string) string {
	matches := pullRequestRegex.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

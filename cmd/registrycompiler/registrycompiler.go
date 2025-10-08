package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/paramsfile"
	"github.com/stackb/centrl/pkg/protoutil"
)

const toolName = "registrycompiler"

type Config struct {
	OutputFile  string
	ModuleFiles []string
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

	if cfg.OutputFile == "" {
		return fmt.Errorf("output_file is required")
	}
	if len(cfg.ModuleFiles) == 0 {
		return fmt.Errorf("deps list must not be empty (positional args)")
	}

	var registry bzpb.Registry

	for _, file := range cfg.ModuleFiles {
		var module bzpb.Module
		if err := protoutil.ReadFile(file, &module); err != nil {
			return fmt.Errorf("reading %s: %v", file, err)
		}
		registry.Modules = append(registry.Modules, &module)
	}

	// Write the compiled ModuleVersion to output file
	if err := protoutil.WriteFile(cfg.OutputFile, &registry); err != nil {
		return fmt.Errorf("failed to write output file: %v", err)
	}

	// log.Printf("Successfully compiled registry: %s", cfg.OutputFile)
	return nil
}

func parseFlags(args []string) (cfg Config, err error) {
	fs := flag.NewFlagSet(toolName, flag.ExitOnError)
	fs.StringVar(&cfg.OutputFile, "output_file", "", "the output file to write")
	fs.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s @PARAMS_FILE", toolName)
		fs.PrintDefaults()
	}

	if err = fs.Parse(args); err != nil {
		return
	}

	cfg.ModuleFiles = fs.Args()

	return
}

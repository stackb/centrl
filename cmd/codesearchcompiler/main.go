package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/junkblocker/codesearch/index"
)

type Config struct {
	Files      []string
	OutputFile string
}

func main() {
	log.SetPrefix("codesearchcompiler: ")
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return fmt.Errorf("failed to parse args: %w", err)
	}

	iw := index.Create(cfg.OutputFile)
	for _, file := range cfg.Files {
		iw.AddFile(file)
	}
	iw.Flush()
	iw.Close()

	return nil
}

func parseFlags(args []string) (cfg Config, err error) {
	fs := flag.NewFlagSet("codesearchcompiler", flag.ExitOnError)
	fs.StringVar(&cfg.OutputFile, "output_file", "", "the output file to write")

	if err = fs.Parse(args); err != nil {
		return
	}

	cfg.Files = fs.Args()

	if cfg.OutputFile == "" {
		return cfg, fmt.Errorf("output_file is required")
	}

	if len(cfg.Files) == 0 {
		return cfg, fmt.Errorf("at least one asset is required")
	}

	return
}

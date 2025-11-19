package bcr

import (
	"log"

	"github.com/bazelbuild/bazel-gazelle/config"
)

const (
	bcrLangName = "bcr"
)

// Config represents the config extension for the a bcr package.
type Config struct {
	config  *config.Config
	enabled bool
}

// createConfig initializes a new Config.
func createConfig(config *config.Config) *Config {
	return &Config{
		config: config,
	}
}

// mustGetConfig returns the scala config.  should only be used after .Configure().
// never nil.
func mustGetConfig(config *config.Config) *Config {
	if existingExt, ok := config.Exts[bcrLangName]; ok {
		return existingExt.(*Config)
	}
	log.Panicln("bcr config nil.  this is a bug")
	return nil
}

// getOrCreateScalaConfig either inserts a new config into the map under the
// language name or replaces it with a clone.
func getOrCreateConfig(config *config.Config) *Config {
	var cfg *Config
	if existingExt, ok := config.Exts[bcrLangName]; ok {
		cfg = existingExt.(*Config).clone(config)
	} else {
		cfg = createConfig(config)
	}
	config.Exts[bcrLangName] = cfg
	return cfg
}

// clone copies this config to a new one.
func (c *Config) clone(config *config.Config) *Config {
	clone := createConfig(config)
	clone.enabled = c.enabled
	return clone
}

// Config returns the parent gazelle configuration
func (c *Config) Config() *config.Config {
	return c.config
}

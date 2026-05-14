package config

import (
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	DataSource          string `yaml:",default=file:a2a_platform.db?cache=shared&_journal_mode=WAL"`
	AgentRetryMax       int    `yaml:",default=3"`
	AgentRetryBaseDelay int    `yaml:",default=1"`
}

// MustLoad loads configuration from the given YAML file path. Panics on error.
func MustLoad(path string) *Config {
	var c Config
	conf.MustLoad(path, &c)
	return &c
}

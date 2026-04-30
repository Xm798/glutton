package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir  string
	Listen   string
	LogLevel string
	TZ       string
}

func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "glutton.db")
}

var validLogLevels = map[string]struct{}{
	"debug": {}, "info": {}, "warn": {}, "error": {},
}

func Load() (Config, error) {
	c := Config{
		DataDir:  envOr("GLUTTON_DATA_DIR", "./data"),
		Listen:   envOr("GLUTTON_LISTEN", ":7890"),
		LogLevel: envOr("GLUTTON_LOG_LEVEL", "info"),
		TZ:       envOr("TZ", "Asia/Shanghai"),
	}
	if _, ok := validLogLevels[c.LogLevel]; !ok {
		return c, fmt.Errorf("invalid GLUTTON_LOG_LEVEL %q", c.LogLevel)
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

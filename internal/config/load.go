package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// Load reads the config file at path, then parses, normalises and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}
	normalise(cfg, filepath.Dir(absPath))

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config %s is invalid:\n%w", path, err)
	}
	return cfg, nil
}

// parse decodes YAML strictly: unknown fields are rejected.
func parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("file is empty")
		}
		return nil, err
	}
	return &cfg, nil
}

// normalise fills in defaults and makes every service directory absolute.
// Relative directories are resolved from the config file's folder,
// not from the folder stackrun was started in.
func normalise(cfg *Config, baseDir string) {
	for name, svc := range cfg.Services {
		if svc == nil {
			continue // reported by Validate
		}

		svc.Name = name

		if svc.Restart == "" {
			svc.Restart = RestartNever
		}

		if svc.StopTimeout == 0 {
			svc.StopTimeout = DefaultStopTimeout
		}

		switch {
		case svc.Dir == "":
			svc.Dir = baseDir
		case !filepath.IsAbs(svc.Dir):
			svc.Dir = filepath.Join(baseDir, svc.Dir)
		}
	}
}

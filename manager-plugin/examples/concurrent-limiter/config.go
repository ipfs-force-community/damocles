package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func LoadConfig(ctx context.Context, pluginsDir string, genDefault bool) (*Config, error) {
	configPath := filepath.Join(pluginsDir, "concurrent-limiter.toml")

	config := defaultConfig(pluginsDir)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := config.GenFile(ctx, configPath); err != nil {
			return nil, fmt.Errorf("generate default config file: %w", err)
		}
	} else {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", configPath, err)
		}
		if err := config.UnmarshalConfig(data); err != nil {
			return nil, err
		}
	}

	return config, nil
}

func defaultConfig(pluginsDir string) *Config {
	return &Config{
		DataDir: filepath.Join(pluginsDir, ".concurrent-limiter"),
		Concurrent: map[string]uint16{
			"add_pieces": 1024,
		},
	}
}

type CommentAll interface {
	CommentAllInExample()
}

type Config struct {
	DataDir    string
	Concurrent map[string]uint16
}

// comment all content
func (Config) CommentAllInExample() {}

func (c *Config) UnmarshalConfig(data []byte) error {
	_, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&c)
	if err != nil {
		return fmt.Errorf("toml.Unmarshal to primitive: %w", err)
	}
	return nil
}

func (c Config) GenFile(ctx context.Context, path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return fmt.Errorf("%s already exits", path)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("stat file %s: %w", path, err)
	}

	content, err := ConfigComment(c)
	if err != nil {
		return fmt.Errorf("marshal default content: %w", err)
	}

	return os.WriteFile(path, content, 0644)
}

func ConfigComment(t interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("# Default config:\n")
	e := toml.NewEncoder(buf)
	e.Indent = ""
	if err := e.Encode(t); err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	b := buf.Bytes()
	b = bytes.ReplaceAll(b, []byte("\n"), []byte("\n#"))

	_, yes := t.(CommentAll)
	if !yes {
		b = bytes.ReplaceAll(b, []byte("#["), []byte("["))
	}

	return b, nil
}

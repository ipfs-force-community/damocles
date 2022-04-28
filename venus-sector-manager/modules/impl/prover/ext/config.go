package ext

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
)

const ConfigKey = "ext-prover"

func defaultProcessorConfig(example bool) ProcessorConfig {
	pcfg := ProcessorConfig{
		Args:       []string{},
		Envs:       map[string]string{},
		Concurrent: 1,
		Weight:     1,
	}

	if example {
		bin := "venus-sector-manger"
		pcfg.Bin = &bin
		pcfg.Args = append(pcfg.Args, ProcessorNameWindostPoSt)
		pcfg.Envs["KEY"] = "VAL"
	}

	return pcfg
}

type ProcessorConfig struct {
	Bin        *string
	Args       []string
	Envs       map[string]string
	Concurrent uint
	Weight     uint
}

func DefaultConfig(example bool) Config {
	cfg := Config{
		WdPost: []ProcessorConfig{},
	}

	if example {
		cfg.WdPost = append(cfg.WdPost, defaultProcessorConfig(example))
	}

	return cfg
}

type Config struct {
	WdPost []ProcessorConfig
}

func (c *Config) UnmarshalConfig(data []byte) error {
	primitive := struct {
		WdPost []toml.Primitive
	}{}

	meta, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&primitive)
	if err != nil {
		return fmt.Errorf("toml.Unmarshal to primitive: %w", err)
	}

	wdposts := make([]ProcessorConfig, 0, len(primitive.WdPost))
	for i, pm := range primitive.WdPost {
		pcfg := defaultProcessorConfig(false)
		err := meta.PrimitiveDecode(pm, &pcfg)
		if err != nil {
			return fmt.Errorf("decode primitive to processor config #%d: %w", i, err)
		}

		wdposts = append(wdposts, pcfg)
	}

	c.WdPost = wdposts
	return nil
}

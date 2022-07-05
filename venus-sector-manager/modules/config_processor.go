package modules

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/extproc"
)

const ProcessorConfigKey = "ext-prover"

type ProcessorConfig struct {
	WdPost  []extproc.ExtProcessorConfig
	WinPost []extproc.ExtProcessorConfig
}

func DefaultProcessorConfig(example bool) ProcessorConfig {
	cfg := ProcessorConfig{
		WdPost:  []extproc.ExtProcessorConfig{},
		WinPost: []extproc.ExtProcessorConfig{},
	}

	if example {
		cfg.WdPost = append(cfg.WdPost, extproc.DefaultExtProcessorConfig(example))
	}

	return cfg
}

func (c *ProcessorConfig) UnmarshalConfig(data []byte) error {
	primitive := struct {
		WdPost  []toml.Primitive
		WinPost []toml.Primitive
	}{}

	meta, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&primitive)
	if err != nil {
		return fmt.Errorf("toml.Unmarshal to primitive: %w", err)
	}

	extractExtConfig := func(prims []toml.Primitive) ([]extproc.ExtProcessorConfig, error) {
		cfgs := make([]extproc.ExtProcessorConfig, 0, len(prims))
		for i, pm := range prims {
			pcfg := extproc.DefaultExtProcessorConfig(false)
			err := meta.PrimitiveDecode(pm, &pcfg)
			if err != nil {
				return nil, fmt.Errorf("decode primitive to processor config #%d: %w", i, err)
			}

			cfgs = append(cfgs, pcfg)
		}

		return cfgs, nil
	}

	wdposts, err := extractExtConfig(primitive.WdPost)
	if err != nil {
		return fmt.Errorf("extract WdPost cfgs: %w", err)
	}

	winposts, err := extractExtConfig(primitive.WinPost)
	if err != nil {
		return fmt.Errorf("extract WinPost cfgs: %w", err)
	}

	c.WdPost = wdposts
	c.WinPost = winposts
	return nil
}

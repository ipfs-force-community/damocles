package modules

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/extproc"
)

const ProcessorConfigKey = "ext-prover"

type ProcessorConfig struct {
	WdPost []extproc.ExtProcessorConfig
}

func DefaultProcessorConfig(example bool) ProcessorConfig {
	cfg := ProcessorConfig{
		WdPost: []extproc.ExtProcessorConfig{},
	}

	if example {
		cfg.WdPost = append(cfg.WdPost, extproc.DefaultExtProcessorConfig(example))
	}

	return cfg
}

func (c *ProcessorConfig) UnmarshalConfig(data []byte) error {
	primitive := struct {
		WdPost []toml.Primitive
	}{}

	meta, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&primitive)
	if err != nil {
		return fmt.Errorf("toml.Unmarshal to primitive: %w", err)
	}

	wdposts := make([]extproc.ExtProcessorConfig, 0, len(primitive.WdPost))
	for i, pm := range primitive.WdPost {
		pcfg := extproc.DefaultExtProcessorConfig(false)
		err := meta.PrimitiveDecode(pm, &pcfg)
		if err != nil {
			return fmt.Errorf("decode primitive to processor config #%d: %w", i, err)
		}

		wdposts = append(wdposts, pcfg)
	}

	c.WdPost = wdposts
	return nil
}

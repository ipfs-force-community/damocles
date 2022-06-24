package extproc

func DefaultExtProcessorConfig(example bool) ExtProcessorConfig {
	pcfg := ExtProcessorConfig{
		Args:             []string{},
		Envs:             map[string]string{},
		Concurrent:       1,
		Weight:           1,
		ReadyTimeoutSecs: 5,
	}

	if example {
		bin := "venus-worker"
		pcfg.Bin = &bin
		pcfg.Args = append(pcfg.Args, ProcessorNameWindostPoSt)
		pcfg.Envs["KEY"] = "VAL"
	}

	return pcfg
}

type ExtProcessorConfig struct {
	Bin              *string
	Args             []string
	Envs             map[string]string
	Concurrent       uint
	Weight           uint
	ReadyTimeoutSecs uint
}

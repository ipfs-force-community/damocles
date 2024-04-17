package extproc

// nolint: flag-parameter
func DefaultExtProcessorConfig(example bool) ExtProcessorConfig {
	pcfg := ExtProcessorConfig{
		Args:             []string{},
		Envs:             map[string]string{},
		Concurrent:       1,
		Weight:           1,
		ReadyTimeoutSecs: 5,
	}

	if example {
		bin := "/path/to/custom/bin"
		pcfg.Bin = &bin
		pcfg.Args = []string{"args1", "args2", "args3"}
		pcfg.Envs["ENV_KEY"] = "ENV_VAL"
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

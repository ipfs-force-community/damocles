package logging

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
)

type ZapLogger = zap.SugaredLogger
type WrappedLogger = logging.ZapEventLogger

var (
	New         = logging.Logger
	SetLogLevel = logging.SetLogLevel
	Nop         = zap.NewNop().Sugar()
)

func Setup() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", "INFO")
	}

	SetLogLevelIfNotSet("dix", "INFO")
	SetLogLevelIfNotSet("kv", "INFO")
	SetLogLevelIfNotSet("rpc", "INFO")
	SetLogLevelIfNotSet("badger", "INFO")

	// copy from lotus
	SetLogLevelIfNotSet("dht", "ERROR")
	SetLogLevelIfNotSet("swarm2", "WARN")
	SetLogLevelIfNotSet("bitswap", "WARN")
	SetLogLevelIfNotSet("connmgr", "WARN")
	SetLogLevelIfNotSet("advmgr", "DEBUG")
	SetLogLevelIfNotSet("stores", "DEBUG")
	SetLogLevelIfNotSet("nat", "INFO")
}

func SetLogLevelIfNotSet(name, level string) {
	_, ok := logging.GetConfig().SubsystemLevels[name]
	if !ok {
		logging.SetLogLevel(name, level)
	}
}

func SetupForSub(system ...string) {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", "ERROR")
		for _, one := range system {
			_ = logging.SetLogLevel(one, "INFO")
		}
	}
}

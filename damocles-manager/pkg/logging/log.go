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
		_ = logging.SetLogLevel("dix", "INFO")
		_ = logging.SetLogLevel("badger", "INFO")
		_ = logging.SetLogLevel("rpc", "INFO")

		// copy from lotus
		_ = logging.SetLogLevel("dht", "ERROR")
		_ = logging.SetLogLevel("swarm2", "WARN")
		_ = logging.SetLogLevel("bitswap", "WARN")
		_ = logging.SetLogLevel("connmgr", "WARN")
		_ = logging.SetLogLevel("advmgr", "DEBUG")
		_ = logging.SetLogLevel("stores", "DEBUG")
		_ = logging.SetLogLevel("nat", "INFO")
	}

	// Always mute kv log
	_ = logging.SetLogLevel("kv", "INFO")
}

func SetupForSub(system ...string) {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", "ERROR")
		for _, one := range system {
			_ = logging.SetLogLevel(one, "INFO")
		}
	}
}

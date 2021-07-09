package logging

import (
	"os"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
)

type ZapLogger = zap.SugaredLogger
type WrappedLogger = logging.ZapEventLogger

var New = logging.Logger

func Setup() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("*", "INFO")
	}
}

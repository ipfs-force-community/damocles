package sealing

import (
	"github.com/dtynn/venus-cluster/venus-sealer/logging"
)

var log = logging.New("sealing")

type ChainServiceAPI interface {
}

type MessageServiceAPI interface {
}

type ServiceAPI interface {
	ChainServiceAPI
	MessageServiceAPI
}

type Sealer struct {
}

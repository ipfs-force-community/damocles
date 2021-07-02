package sealing

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

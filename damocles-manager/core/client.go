package core

var UnavailableAPIClient = APIClient{
	SealerAPIClient:       UnavailableSealerAPIClient,
	SealerCliAPIClient:    UnavailableSealerCliAPIClient,
	RandomnessAPIClient:   UnavailableRandomnessAPIClient,
	MinerAPIClient:        UnavailableMinerAPIClient,
	WorkerWdPoStAPIClient: UnavailableWorkerWdPoStAPIClient,
}

type APIClient struct {
	SealerAPIClient
	SealerCliAPIClient
	RandomnessAPIClient
	MinerAPIClient
	WorkerWdPoStAPIClient
}

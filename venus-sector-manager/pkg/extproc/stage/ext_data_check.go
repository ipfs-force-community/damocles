package stage

const NameDataCheck = "data_check"

type TransferStoreInfo struct {
	Name string            `json:"name"`
	Meta map[string]string `json:"meta"`
}

type DataCheckItem struct {
	StoreName *string `json:"store_name"`
	URI       string  `json:"uri"`
	Size      *uint64 `json:"size"`
}

type DataCheckFailure struct {
	Item    DataCheckItem `json:"item"`
	Failure string        `json:"failure"`
}

type DataCheck struct {
	Stores map[string]TransferStoreInfo `json:"stores"`
	Items  []DataCheckItem              `json:"items"`
}

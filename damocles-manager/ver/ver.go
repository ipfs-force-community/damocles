package ver

import "fmt"

const Version = "0.12.3"

var Commit string

func VersionStr() string {
	return fmt.Sprintf("v%s-%s-%s", Version, Prover, Commit)
}

func ProverIsProd() bool {
	return Prover == "prod"
}

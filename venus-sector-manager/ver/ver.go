package ver

import "fmt"

const Version = "0.4.0-rc5"

var Commit string

func VersionStr() string {
	return fmt.Sprintf("v%s-%s-%s", Version, Prover, Commit)
}

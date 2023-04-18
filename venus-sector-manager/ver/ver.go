package ver

import "fmt"

const Version = "0.6.0-rc2"

var Commit string

func VersionStr() string {
	return fmt.Sprintf("v%s-%s-%s", Version, Prover, Commit)
}

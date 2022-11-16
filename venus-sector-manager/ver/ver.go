package ver

import "fmt"

const Version = "0.5.0-beta3"

var Commit string

func VersionStr() string {
	return fmt.Sprintf("v%s-%s-%s", Version, Prover, Commit)
}

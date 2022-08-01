package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

var (
	NewKey = tag.NewKey
	New    = tag.New
	Upsert = tag.Upsert

	Record = stats.Record
)

type Key = tag.Key

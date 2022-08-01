package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100000)
var winningPostSecondsDistribution = view.Distribution(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 100)
var windowPostMinutesDistribution = view.Distribution(1, 5, 10, 15, 20, 25, 30)

var (
	Endpoint, _     = tag.NewKey("endpoint")
	APIInterface, _ = tag.NewKey("api") // to distinguish between gateway api and full node api endpoint calls
	Miner, _        = tag.NewKey("miner")
)

var (
	// common
	VenusClusterInfo = stats.Int64("info", "Arbitrary counter to tag venus cluster info", stats.UnitDimensionless)

	SectorManagerNewSector = stats.Int64("sector_manager/new_sector", "Count the new sector", stats.UnitDimensionless)

	SectorManagerPreCommitSector = stats.Int64("sector_manager/pre_commit_sector", "Count the pre commit sector", stats.UnitDimensionless)

	SectorManagerCommitSector = stats.Int64("sector_manager/commit_sector", "Count the commit sector", stats.UnitDimensionless)

	ProverWinningPostDuration = stats.Float64("prover/winning_post_duration", "Duration of winning post ", stats.UnitDimensionless)

	ProverWindowPostDuration = stats.Float64("prover/window_post_duration", "Duration of window post", stats.UnitDimensionless)

	ProverWindowPostCompleteRate = stats.Float64("prover/window_post_complete_rate", "Complete rate of window post", stats.UnitDimensionless)

	APIRequestDuration = stats.Float64("api/request_duration_ms", "Duration of API requests", stats.UnitMilliseconds)
)

var (
	InfoView = &view.View{
		Name:        "info",
		Description: "venus cluster information",
		Measure:     VenusClusterInfo,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{},
	}

	NewSectorView = &view.View{
		Name:        "new_sector",
		Description: "count of sector manager new sector",
		TagKeys:     []tag.Key{Miner},
		Measure:     SectorManagerNewSector,
		Aggregation: view.Count(),
	}

	SectorPreCommitView = &view.View{
		Name:        "sector_pre_commit",
		Description: "count of sector manager pre commit sector",
		TagKeys:     []tag.Key{Miner},
		Measure:     SectorManagerPreCommitSector,
		Aggregation: view.Count(),
	}

	SectorCommitView = &view.View{
		Name:        "sector_commit",
		Description: "count of sector manager commit sector",
		TagKeys:     []tag.Key{Miner},
		Measure:     SectorManagerCommitSector,
		Aggregation: view.Count(),
	}

	ProverWinningPostDurationView = &view.View{
		TagKeys:     []tag.Key{Miner},
		Measure:     ProverWinningPostDuration,
		Aggregation: winningPostSecondsDistribution,
	}

	ProverWindowPostDurationView = &view.View{
		TagKeys:     []tag.Key{Miner},
		Measure:     ProverWindowPostDuration,
		Aggregation: windowPostMinutesDistribution,
	}

	ProverWindowPostCompleteRateView = &view.View{
		TagKeys:     []tag.Key{Miner},
		Measure:     ProverWindowPostCompleteRate,
		Aggregation: view.LastValue(),
	}

	APIRequestDurationView = &view.View{
		Measure:     APIRequestDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{APIInterface, Endpoint},
	}
)

var VenusClusterViews = []*view.View{
	InfoView,
	NewSectorView,
	APIRequestDurationView,
	SectorPreCommitView,
	SectorCommitView,
	ProverWinningPostDurationView,
	ProverWindowPostDurationView,
	ProverWindowPostCompleteRateView,
}

type TimeParser func(time.Time) float64

func Timer(ctx context.Context, m *stats.Float64Measure, fn TimeParser) func() {
	start := time.Now()
	return func() {
		stats.Record(ctx, m.M(fn(start)))
	}
}

func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e6
}

func SinceInSeconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e9
}

func SinceInMinutes(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e9 / 60
}

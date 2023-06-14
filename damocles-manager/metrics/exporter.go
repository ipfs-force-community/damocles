package metrics

import (
	"context"
	"net/http"

	"contrib.go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

const logMetrics = "metrics"

var log = logging.New(logMetrics)

func Exporter() http.Handler {
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "damocles_manager",
	})
	if err != nil {
		log.Errorf("could not create the prometheus stats exporter: %v", err)
	}

	if err := view.Register(DamoclesViews...); err != nil {
		panic(err)
	}

	stats.Record(context.Background(), VenusClusterInfo.M(int64(1)))
	return exporter
}

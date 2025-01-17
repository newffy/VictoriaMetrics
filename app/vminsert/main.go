package vminsert

import (
	"flag"
	"fmt"
	opentsdbhttp "github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/opentsdb-http"
	"net/http"
	"strings"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/concurrencylimiter"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/graphite"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/influx"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/opentsdb"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/prometheus"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/httpserver"
	"github.com/VictoriaMetrics/metrics"
)

var (
	graphiteListenAddr   = flag.String("graphiteListenAddr", "", "TCP and UDP address to listen for Graphite plaintext data. Usually :2003 must be set. Doesn't work if empty")
	opentsdbListenAddr   = flag.String("opentsdbListenAddr", "", "TCP and UDP address to listen for OpentTSDB put messages. Usually :4242 must be set. Doesn't work if empty")
	maxInsertRequestSize = flag.Int("maxInsertRequestSize", 32*1024*1024, "The maximum size of a single insert request in bytes")
)

// Init initializes vminsert.
func Init() {
	concurrencylimiter.Init()
	if len(*graphiteListenAddr) > 0 {
		go graphite.Serve(*graphiteListenAddr)
	}
	if len(*opentsdbListenAddr) > 0 {
		go opentsdb.Serve(*opentsdbListenAddr)
	}
}

// Stop stops vminsert.
func Stop() {
	if len(*graphiteListenAddr) > 0 {
		graphite.Stop()
	}
	if len(*opentsdbListenAddr) > 0 {
		opentsdb.Stop()
	}
}

// RequestHandler is a handler for Prometheus remote storage write API
func RequestHandler(w http.ResponseWriter, r *http.Request) bool {
	path := strings.Replace(r.URL.Path, "//", "/", -1)
	switch path {
	case "/api/v1/write":
		prometheusWriteRequests.Inc()
		if err := prometheus.InsertHandler(r, int64(*maxInsertRequestSize)); err != nil {
			prometheusWriteErrors.Inc()
			httpserver.Errorf(w, "error in %q: %s", r.URL.Path, err)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	case "/write", "/api/v2/write":
		influxWriteRequests.Inc()
		if err := influx.InsertHandler(r); err != nil {
			influxWriteErrors.Inc()
			httpserver.Errorf(w, "error in %q: %s", r.URL.Path, err)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	case "/query":
		// Emulate fake response for influx query.
		// This is required for TSBS benchmark.
		influxQueryRequests.Inc()
		fmt.Fprintf(w, `{"results":[{"series":[{"values":[]}]}]}`)
		return true
	case "/api/put":
		opentsdbHttpWriteRequests.Inc()
		if err := opentsdbhttp.InsertHandler(r, int64(*maxInsertRequestSize)); err != nil {
			opentsdbHttpWriteErrors.Inc()
			httpserver.Errorf(w, "error in %q: %s", r.URL.Path, err)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	default:
		// This is not our link
		return false
	}
}

var (
	prometheusWriteRequests = metrics.NewCounter(`vm_http_requests_total{path="/api/v1/write", protocol="prometheus"}`)
	prometheusWriteErrors   = metrics.NewCounter(`vm_http_request_errors_total{path="/api/v1/write", protocol="prometheus"}`)

	influxWriteRequests = metrics.NewCounter(`vm_http_requests_total{path="/write", protocol="influx"}`)
	influxWriteErrors   = metrics.NewCounter(`vm_http_request_errors_total{path="/write", protocol="influx"}`)

	influxQueryRequests = metrics.NewCounter(`vm_http_requests_total{path="/query", protocol="influx"}`)

	opentsdbHttpWriteRequests = metrics.NewCounter(`vm_http_requests_total{path="/api/put", protocol="opentsdb-http"}`)
	opentsdbHttpWriteErrors   = metrics.NewCounter(`vm_http_request_errors_total{path="/api/put", protocol="opentsdb-http"}`)

)

package otlp

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func postFixture(t *testing.T, r *receiver, name string) {
	t.Helper()
	if got := r.post(t, "/v1/metrics", fixture(t, name), "Content-Type", contentProtobuf); got.status != 200 {
		t.Fatalf("%s: status %d, want 200", name, got.status)
	}
}

func byMetric(lines []Line, name string) []Signal {
	var out []Signal
	for _, l := range lines {
		if l.Signal.Kind == contract.SignalKindMetric && l.Signal.Metric.Name == name {
			out = append(out, l.Signal)
		}
	}
	return out
}

func TestACumulativeSumKeepsOneSeriesWhoseRevisionIsThePointTime(t *testing.T) {
	r := newReceiver(t)
	postFixture(t, r, "sdk-metrics-cumulative-1.bin")
	postFixture(t, r, "sdk-metrics-cumulative-2.bin")
	points := byMetric(r.lines(t), "toy.requests")
	if len(points) != 2 {
		t.Fatalf("%d toy.requests points, want one per export", len(points))
	}
	first, second := points[0], points[1]
	if !strings.HasPrefix(first.ID, "metric:") || len(first.ID) != len("metric:")+32 || first.ID != second.ID {
		t.Fatalf("ids %q and %q, want one metric: series id", first.ID, second.ID)
	}
	if first.Metric.Value != 3 || second.Metric.Value != 5 || first.Metric.Type != contract.MetricTypeSum || first.Metric.Unit != "{request}" {
		t.Fatalf("bodies %+v %+v, want the running totals 3 then 5", first.Metric, second.Metric)
	}
	for _, p := range points {
		ts, err := timeOf(p.TS)
		if err != nil || p.Rev != int(ts.UnixMilli()) || p.Rev == 0 {
			t.Fatalf("rev %d for ts %q, want the point time in Unix milliseconds", p.Rev, p.TS)
		}
	}
	if second.Rev <= first.Rev {
		t.Fatalf("revs %d then %d, want a rising revision", first.Rev, second.Rev)
	}
	if first.Attrs["route"] != "/items" || first.Attrs["method"] != "GET" || first.Attrs[ServiceAttr] != "toy-api" {
		t.Fatalf("attributes %v", first.Attrs)
	}
}

func TestASeriesIDDependsOnServiceNameAndAttributesButNotTheirOrder(t *testing.T) {
	a, b := str("route", "/items"), str("method", "GET")
	base := seriesID("toy-api", "toy.requests", []*commonv1.KeyValue{a, b})
	if base != seriesID("toy-api", "toy.requests", []*commonv1.KeyValue{b, a}) {
		t.Fatal("the attribute order changed the series id")
	}
	others := []string{
		seriesID("toy-worker", "toy.requests", []*commonv1.KeyValue{a, b}),
		seriesID("toy-api", "toy.errors", []*commonv1.KeyValue{a, b}),
		seriesID("toy-api", "toy.requests", []*commonv1.KeyValue{a}),
		seriesID("toy-api", "toy.requests", []*commonv1.KeyValue{a, str("method", "POST")}),
		seriesID("toy-ap", "itoy.requests", []*commonv1.KeyValue{a, b}),
	}
	for i, o := range others {
		if o == base {
			t.Fatalf("variant %d kept the series id %q", i, base)
		}
	}
}

func TestADeltaSumStaysAppendOnly(t *testing.T) {
	r := newReceiver(t)
	postFixture(t, r, "sdk-metrics-delta.bin")
	postFixture(t, r, "sdk-metrics-delta.bin")
	points := byMetric(r.lines(t), "toy.jobs")
	if len(points) != 2 {
		t.Fatalf("%d toy.jobs points, want 2", len(points))
	}
	for _, p := range points {
		if p.ID != "" || p.Rev != 0 || p.Metric.Value != 5 || p.Metric.Type != contract.MetricTypeSum {
			t.Fatalf("delta point %+v %+v, want no id, no rev and its own value", p, p.Metric)
		}
	}
	cumulative := newReceiver(t)
	postFixture(t, cumulative, "sdk-metrics-cumulative-1.bin")
	if p := byMetric(cumulative.lines(t), "toy.requests"); len(p) != 1 || p[0].ID == "" {
		t.Fatalf("cumulative canary %+v, want a series id", p)
	}
}

func TestAHistogramKeepsBoundsAndCounts(t *testing.T) {
	r := newReceiver(t)
	postFixture(t, r, "sdk-metrics-cumulative-1.bin")
	points := byMetric(r.lines(t), "toy.latency")
	if len(points) != 1 {
		t.Fatalf("%d toy.latency points, want 1", len(points))
	}
	p := points[0]
	h := p.Metric.Histogram
	if p.Metric.Type != contract.MetricTypeHistogram || h == nil || p.ID == "" || p.Metric.Unit != "ms" {
		t.Fatalf("histogram %+v, want a histogram series in ms", p.Metric)
	}
	if h.Count != 3 || h.Sum != 86 || p.Metric.Value != 86 || !slices.Equal(h.Bounds, []float64{5, 10, 25, 50}) || !slices.Equal(h.Counts, []uint64{1, 0, 1, 0, 1}) {
		t.Fatalf("histogram body %+v value %v", h, p.Metric.Value)
	}
}

func timeOf(ts string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, ts)
}

func metricExport(ms ...*metricsv1.Metric) *metricsv1.MetricsData {
	return &metricsv1.MetricsData{ResourceMetrics: []*metricsv1.ResourceMetrics{{
		Resource:     resource("toy-api"),
		ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: ms}},
	}}}
}

func TestOtherMetricShapesKeepCountAndSumAndUnfitBucketsAreLeftOut(t *testing.T) {
	r := newReceiver(t)
	sum := 12.5
	at := uint64(1_794_457_309_000_000_000)
	md := metricExport(
		&metricsv1.Metric{Name: "toy.size", Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: &metricsv1.ExponentialHistogram{
			AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
			DataPoints:             []*metricsv1.ExponentialHistogramDataPoint{{TimeUnixNano: at, Count: 4, Sum: &sum}},
		}}},
		&metricsv1.Metric{Name: "toy.quantiles", Data: &metricsv1.Metric_Summary{Summary: &metricsv1.Summary{
			DataPoints: []*metricsv1.SummaryDataPoint{{TimeUnixNano: at, Count: 7, Sum: 21}},
		}}},
		&metricsv1.Metric{Name: "toy.uneven", Data: &metricsv1.Metric_Histogram{Histogram: &metricsv1.Histogram{
			AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
			DataPoints:             []*metricsv1.HistogramDataPoint{{TimeUnixNano: at, Count: 2, Sum: &sum, ExplicitBounds: []float64{10, 5}, BucketCounts: []uint64{1, 0, 1}}},
		}}},
		&metricsv1.Metric{Name: "toy.load", Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{
			DataPoints: []*metricsv1.NumberDataPoint{{TimeUnixNano: at, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 0.75}}, {TimeUnixNano: at, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: math.NaN()}}},
		}}},
	)
	if got := r.postProto(t, "/v1/metrics", md); got.status != 200 {
		t.Fatalf("status %d", got.status)
	}
	lines := r.lines(t)
	if len(lines) != 4 {
		t.Fatalf("%d lines, want 4 with the NaN gauge point dropped", len(lines))
	}
	exp := byMetric(lines, "toy.size")[0]
	if exp.ID != "" || exp.Metric.Type != contract.MetricTypeHistogram || exp.Metric.Histogram.Count != 4 || exp.Metric.Histogram.Sum != 12.5 || exp.Metric.Histogram.Bounds != nil {
		t.Fatalf("delta exponential histogram %+v %+v", exp, exp.Metric.Histogram)
	}
	summary := byMetric(lines, "toy.quantiles")[0]
	if summary.ID == "" || summary.Metric.Type != contract.MetricTypeSummary || summary.Metric.Value != 21 || summary.Metric.Histogram != nil || summary.Attrs["count"] != float64(7) {
		t.Fatalf("summary %+v %+v", summary, summary.Metric)
	}
	uneven := byMetric(lines, "toy.uneven")[0].Metric.Histogram
	if uneven.Count != 2 || uneven.Bounds != nil || uneven.Counts != nil {
		t.Fatalf("histogram with descending bounds %+v, want count and sum only", uneven)
	}
	gauge := byMetric(lines, "toy.load")
	if len(gauge) != 1 || gauge[0].ID == "" || gauge[0].Metric.Type != contract.MetricTypeGauge || gauge[0].Metric.Value != 0.75 {
		t.Fatalf("gauge %+v", gauge)
	}
	if !strings.Contains(r.log.String(), `"dropped":1`) {
		t.Fatalf("the dropped point is not logged: %q", r.log.String())
	}
}

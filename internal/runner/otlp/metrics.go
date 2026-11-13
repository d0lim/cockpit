package otlp

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"
	"strings"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/d0lim/cockpit/internal/contract"
)

const seriesPrefix = "metric:"

type point struct {
	attrs []*commonv1.KeyValue
	time  uint64
	value float64
	hist  *contract.HistogramBody
	count *uint64
	unfit bool
}

type drops struct {
	points  int
	buckets int
}

type shape struct {
	typ    contract.MetricType
	series bool
	points []point
}

func metricSignals(md *metricsv1.MetricsData) (out []Signal, dropped drops, err error) {
	n := 0
	for _, rm := range md.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				n += pointCount(m)
			}
		}
	}
	if n > MaxSignals {
		return nil, drops{}, errTooMany
	}
	out = make([]Signal, 0, n)
	for _, rm := range md.GetResourceMetrics() {
		service := serviceOf(rm.GetResource())
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				sh := shapeOf(m)
				for _, p := range sh.points {
					if !finite(p) {
						dropped.points++
						continue
					}
					if p.unfit {
						dropped.buckets++
					}
					out = append(out, metricSignal(m, sh, p, service))
				}
			}
		}
	}
	return out, dropped, nil
}

func pointCount(m *metricsv1.Metric) int {
	switch d := m.GetData().(type) {
	case *metricsv1.Metric_Gauge:
		return len(d.Gauge.GetDataPoints())
	case *metricsv1.Metric_Sum:
		return len(d.Sum.GetDataPoints())
	case *metricsv1.Metric_Histogram:
		return len(d.Histogram.GetDataPoints())
	case *metricsv1.Metric_ExponentialHistogram:
		return len(d.ExponentialHistogram.GetDataPoints())
	case *metricsv1.Metric_Summary:
		return len(d.Summary.GetDataPoints())
	}
	return 0
}

func metricSignal(m *metricsv1.Metric, sh shape, p point, service string) Signal {
	attrs := attrsOf(p.attrs, service)
	if p.count != nil {
		attrs["count"] = *p.count
	}
	sig := Signal{
		Kind:     contract.SignalKindMetric,
		TS:       stamp(p.time),
		TSSource: contract.TimeSourceContainer,
		Attrs:    attrs,
		Metric:   &contract.MetricBody{Name: m.GetName(), Value: p.value, Unit: m.GetUnit(), Type: sh.typ, Histogram: p.hist},
	}
	if sh.series {
		sig.ID = seriesID(service, m.GetName(), p.attrs)
		sig.Rev = int(p.time / 1e6)
	}
	return sig
}

func cumulative(t metricsv1.AggregationTemporality) bool {
	return t == metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE
}

func shapeOf(m *metricsv1.Metric) shape {
	switch d := m.GetData().(type) {
	case *metricsv1.Metric_Gauge:
		return shape{typ: contract.MetricTypeGauge, series: true, points: numberPoints(d.Gauge.GetDataPoints())}
	case *metricsv1.Metric_Sum:
		return shape{typ: contract.MetricTypeSum, series: cumulative(d.Sum.GetAggregationTemporality()), points: numberPoints(d.Sum.GetDataPoints())}
	case *metricsv1.Metric_Histogram:
		var ps []point
		for _, dp := range d.Histogram.GetDataPoints() {
			h := &contract.HistogramBody{Count: dp.GetCount(), Sum: dp.GetSum()}
			unfit := slices.ContainsFunc(dp.GetExplicitBounds(), notFinite)
			if !unfit && buckets(dp.GetExplicitBounds(), dp.GetBucketCounts()) {
				h.Bounds, h.Counts = dp.GetExplicitBounds(), dp.GetBucketCounts()
			}
			ps = append(ps, point{attrs: dp.GetAttributes(), time: dp.GetTimeUnixNano(), value: h.Sum, hist: h, unfit: unfit})
		}
		return shape{typ: contract.MetricTypeHistogram, series: cumulative(d.Histogram.GetAggregationTemporality()), points: ps}
	case *metricsv1.Metric_ExponentialHistogram:
		var ps []point
		for _, dp := range d.ExponentialHistogram.GetDataPoints() {
			h := &contract.HistogramBody{Count: dp.GetCount(), Sum: dp.GetSum()}
			ps = append(ps, point{attrs: dp.GetAttributes(), time: dp.GetTimeUnixNano(), value: h.Sum, hist: h})
		}
		return shape{typ: contract.MetricTypeHistogram, series: cumulative(d.ExponentialHistogram.GetAggregationTemporality()), points: ps}
	case *metricsv1.Metric_Summary:
		var ps []point
		for _, dp := range d.Summary.GetDataPoints() {
			n := dp.GetCount()
			ps = append(ps, point{attrs: dp.GetAttributes(), time: dp.GetTimeUnixNano(), value: dp.GetSum(), count: &n})
		}
		return shape{typ: contract.MetricTypeSummary, series: true, points: ps}
	}
	return shape{}
}

func numberPoints(dps []*metricsv1.NumberDataPoint) []point {
	out := make([]point, 0, len(dps))
	for _, dp := range dps {
		v := dp.GetAsDouble()
		if i, ok := dp.GetValue().(*metricsv1.NumberDataPoint_AsInt); ok {
			v = float64(i.AsInt)
		}
		out = append(out, point{attrs: dp.GetAttributes(), time: dp.GetTimeUnixNano(), value: v})
	}
	return out
}

func buckets(bounds []float64, counts []uint64) bool {
	if len(counts) != len(bounds)+1 {
		return false
	}
	for i := 1; i < len(bounds); i++ {
		if bounds[i] <= bounds[i-1] {
			return false
		}
	}
	return true
}

func notFinite(f float64) bool {
	return math.IsNaN(f) || math.IsInf(f, 0)
}

func finite(p point) bool {
	if p.hist != nil && notFinite(p.hist.Sum) {
		return false
	}
	return !notFinite(p.value)
}

func seriesID(service, name string, kvs []*commonv1.KeyValue) string {
	sorted := slices.Clone(kvs)
	slices.SortFunc(sorted, func(a, b *commonv1.KeyValue) int { return strings.Compare(a.GetKey(), b.GetKey()) })
	h := sha256.New()
	field := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	field(service)
	field(name)
	for _, kv := range sorted {
		raw, _ := json.Marshal(valueOf(kv.GetValue()))
		field(kv.GetKey())
		field(string(raw))
	}
	return seriesPrefix + hex.EncodeToString(h.Sum(nil)[:16])
}

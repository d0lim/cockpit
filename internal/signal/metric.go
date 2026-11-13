package signal

import (
	"errors"
	"fmt"

	"github.com/d0lim/cockpit/internal/contract"
)

func ValidateMetric(m *contract.MetricBody) error {
	switch m.Type {
	case "", contract.MetricTypeGauge, contract.MetricTypeSum, contract.MetricTypeHistogram, contract.MetricTypeSummary:
	default:
		return fmt.Errorf("unknown metric type %q", m.Type)
	}
	h := m.Histogram
	if (h != nil) != (m.Type == contract.MetricTypeHistogram) {
		return errors.New("a metric has a histogram exactly when its type is histogram")
	}
	if h == nil {
		return nil
	}
	if m.Value != h.Sum {
		return errors.New("a histogram metric's value must equal its sum")
	}
	if len(h.Counts) != len(h.Bounds)+1 && (len(h.Counts) != 0 || len(h.Bounds) != 0) {
		return errors.New("a histogram needs one count more than its bounds, or neither")
	}
	for i := 1; i < len(h.Bounds); i++ {
		if h.Bounds[i] <= h.Bounds[i-1] {
			return errors.New("histogram bounds must ascend")
		}
	}
	return nil
}

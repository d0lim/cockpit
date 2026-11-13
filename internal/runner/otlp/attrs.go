package otlp

import (
	"encoding/base64"
	"math"
	"strconv"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/d0lim/cockpit/internal/contract"
)

func serviceOf(r *resourcev1.Resource) string {
	for _, kv := range r.GetAttributes() {
		if kv.GetKey() == "service.name" {
			return kv.GetValue().GetStringValue()
		}
	}
	return ""
}

func attrsOf(kvs []*commonv1.KeyValue, service string) contract.Attrs {
	out := make(contract.Attrs, len(kvs)+1)
	for _, kv := range kvs {
		out[kv.GetKey()] = valueOf(kv.GetValue())
	}
	out[ServiceAttr] = service
	return out
}

func valueOf(v *commonv1.AnyValue) any {
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	case *commonv1.AnyValue_BoolValue:
		return x.BoolValue
	case *commonv1.AnyValue_IntValue:
		return x.IntValue
	case *commonv1.AnyValue_DoubleValue:
		if math.IsNaN(x.DoubleValue) || math.IsInf(x.DoubleValue, 0) {
			return strconv.FormatFloat(x.DoubleValue, 'g', -1, 64)
		}
		return x.DoubleValue
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(x.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		vals := x.ArrayValue.GetValues()
		out := make([]any, len(vals))
		for i, e := range vals {
			out[i] = valueOf(e)
		}
		return out
	case *commonv1.AnyValue_KvlistValue:
		kvs := x.KvlistValue.GetValues()
		out := make(map[string]any, len(kvs))
		for _, kv := range kvs {
			out[kv.GetKey()] = valueOf(kv.GetValue())
		}
		return out
	}
	return nil
}

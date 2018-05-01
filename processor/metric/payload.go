package metric

import (
	"encoding/json"
	"time"

	"github.com/elastic/apm-server/config"
	m "github.com/elastic/apm-server/model"
	"github.com/elastic/apm-server/utility"
	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
)

var (
	processorEntry = common.MapStr{"name": processorName, "event": docType}
)

type Sample interface {
	Name() string
	Value() interface{}
}

type StandardSample struct {
	name  string
	value interface{}
}

func (c StandardSample) Name() string {
	return c.name
}

func (c StandardSample) Value() interface{} {
	return c.value
}

type Metric struct {
	Samples   []Sample
	Timestamp time.Time
}

type Payload struct {
	Process *m.Process
	Service m.Service
	System  *m.System
	Metrics []Metric
}

func decodeSample(name string, s json.Number) Sample {
	if f, err := s.Int64(); err == nil {
		return StandardSample{
			name:  name,
			value: f,
		}
	}

	if f, err := s.Float64(); err == nil {
		return StandardSample{
			name:  name,
			value: f,
		}
	}

	return StandardSample{
		name:  name,
		value: s.String(),
	}
}

func firstV(sample map[string]interface{}) interface{} {
	for _, k := range []string{"count", "value"} {
		if v, ok := sample[k]; ok {
			return v
		}
	}
	return nil
}

func decodeSamples(raw map[string]interface{}) []Sample {
	sl := make([]Sample, 0)

	samples := raw["samples"].(map[string]interface{})
	for name, s := range samples {
		sample := s.(map[string]interface{})
		v := firstV(sample)
		if v == nil {
			continue
		}
		f, ok := v.(json.Number)
		if !ok {
			continue
		}
		sl = append(sl, decodeSample(name, f))
	}

	return sl
}

func DecodePayload(raw map[string]interface{}) (*Payload, error) {
	if raw == nil {
		return nil, nil
	}
	pa := Payload{}

	var err error
	service, err := m.DecodeService(raw["service"], err)
	if service != nil {
		pa.Service = *service
	}
	pa.System, err = m.DecodeSystem(raw["system"], err)
	pa.Process, err = m.DecodeProcess(raw["process"], err)
	if err != nil {
		return nil, err
	}

	decoder := utility.ManualDecoder{}
	metrics := decoder.InterfaceArr(raw, "metrics")
	for _, m := range metrics {
		raw := m.(map[string]interface{})
		pa.Metrics = append(pa.Metrics, Metric{
			Samples:   decodeSamples(raw),
			Timestamp: decoder.TimeRFC3339WithDefault(raw, "timestamp"),
		})
	}
	return &pa, nil
}

func (pa *Payload) Transform(conf config.Config) []beat.Event {
	if pa == nil {
		return nil
	}

	var events []beat.Event
	for _, metric := range pa.Metrics {
		fields := common.MapStr{
			"processor": processorEntry,
			"process":   pa.Process.Transform(),
			"service":   pa.Service.Transform(),
			"system":    pa.System.Transform(),
		}
		for _, sample := range metric.Samples {
			fields.Put("metric."+sample.Name(), sample.Value())
		}
		ev := beat.Event{
			Fields:    fields,
			Timestamp: time.Now(),
		}
		events = append(events, ev)
	}
	return events
}

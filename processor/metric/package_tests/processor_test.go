package package_tests

import (
	"testing"

	"github.com/elastic/apm-server/config"
	"github.com/elastic/apm-server/processor/metric"
	"github.com/elastic/apm-server/tests"
)

func TestMetricProcessorOK(t *testing.T) {
	requestInfo := []tests.RequestInfo{
		{Name: "TestProcessMetric", Path: "data/valid/metric/payload.json"},
	}
	tests.TestProcessRequests(t, metric.NewProcessor(), config.Config{}, requestInfo, map[string]string{})
}

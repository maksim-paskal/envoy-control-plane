/*
Copyright paskal.maksim@gmail.com
Licensed under the Apache License, Version 2.0 (the "License")
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package metrics_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
)

var (
	client = &http.Client{}
	ts     = httptest.NewServer(metrics.GetHandler())
	ctx    = context.Background()
)

func TestMetricsInc(t *testing.T) {
	t.Parallel()

	metrics.GrpcOnStreamOpen.Inc()
	metrics.GrpcOnStreamClosed.Inc()
	metrics.GrpcOnStreamRequest.Inc()
	metrics.GrpcOnStreamResponse.Inc()
	metrics.GrpcOnFetchRequest.Inc()
	metrics.GrpcOnFetchResponse.Inc()
	metrics.GrpcOnStreamDeltaRequest.Inc()
	metrics.GrpcOnStreamDeltaResponse.Inc()
	metrics.GrpcOnStreamDeltaRequestOnStreamDeltaRequest.Inc()
	metrics.GrpcOnDeltaStreamOpen.Inc()
	metrics.GrpcOnDeltaStreamClosed.Inc()
	metrics.KubernetesAPIRequest.WithLabelValues("200").Inc()
	metrics.KubernetesAPIRequestDuration.Observe(1)
}

func TestMetricsHandler(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if m := "envoy_control_plane_on_delta_stream_closed_total"; !strings.Contains(string(body), m) {
		t.Fatalf("no metric %s found", m)
	}
}

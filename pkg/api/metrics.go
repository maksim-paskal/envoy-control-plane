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
package api

import (
	"net/url"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
)

type requestResult struct{}

func (r *requestResult) Increment(code string, method string, host string) {
	metrics.KubernetesAPIRequest.WithLabelValues(code).Inc()
}

type requestLatency struct{}

func (r *requestLatency) Observe(verb string, u url.URL, latency time.Duration) {
	metrics.KubernetesAPIRequestDuration.Observe(latency.Seconds())
}
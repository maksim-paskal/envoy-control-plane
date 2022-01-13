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
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "envoy_control_plane"

var (
	GrpcOnStreamOpen = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_open_total",
		Help:      "The total number of OnStreamOpen events",
	})
	GrpcOnStreamClosed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_closed_total",
		Help:      "The total number of OnStreamClosed events",
	})
	GrpcOnStreamRequest = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_request_total",
		Help:      "The total number of OnStreamRequest events",
	})
	GrpcOnStreamResponse = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_response_total",
		Help:      "The total number of OnStreamResponse events",
	})
	GrpcOnFetchRequest = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_fetch_request_total",
		Help:      "The total number of OnFetchRequest events",
	})
	GrpcOnFetchResponse = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_fetch_response_total",
		Help:      "The total number of OnFetchResponse events",
	})
	GrpcOnStreamDeltaRequest = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_delta_request_total",
		Help:      "The total number of OnStreamDeltaRequest events",
	})
	GrpcOnStreamDeltaResponse = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_delta_response_total",
		Help:      "The total number of OnStreamDeltaResponse events",
	})
	GrpcOnStreamDeltaRequestOnStreamDeltaRequest = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_stream_delta_request_on_stream_delta_request_total",
		Help:      "The total number of OnStreamDeltaRequestOnStreamDeltaRequest events",
	})
	GrpcOnDeltaStreamOpen = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_delta_stream_open_total",
		Help:      "The total number of OnDeltaStreamOpen events",
	})
	GrpcOnDeltaStreamClosed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "on_delta_stream_closed_total",
		Help:      "The total number of OnDeltaStreamClosed events",
	})
	KubernetesAPIRequest = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "apiserver_request_total",
		Help:      "The total number of kunernetes API requests",
	}, []string{"code"})

	KubernetesAPIRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "apiserver_request_duration",
		Help:      "The duration in seconds of kunernetes API requests",
	})

	ConfigmapsstoreAddFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "configmapsstore_add_total",
		Help:      "The total number of AddFunc events in configmapsstore",
	})

	ConfigmapsstoreUpdateFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "configmapsstore_update_total",
		Help:      "The total number of UpdateFunc events in configmapsstore",
	})

	ConfigmapsstoreDeleteFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "configmapsstore_delete_total",
		Help:      "The total number of DeleteFunc events in configmapsstore",
	})

	EndpointstoreAddFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "endpointstore_add_total",
		Help:      "The total number of AddFunc events in endpointstore",
	})

	EndpointstoreUpdateFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "endpointstore_update_total",
		Help:      "The total number of UpdateFunc events in endpointstore",
	})

	EndpointstoreDeleteFunc = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "endpointstore_delete_total",
		Help:      "The total number of DeleteFunc events in endpointstore",
	})
)

func GetHandler() http.Handler {
	return promhttp.Handler()
}

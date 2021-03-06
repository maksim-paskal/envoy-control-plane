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
package main

import (
	"context"
	"sync"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

func (cb *callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.WithFields(log.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("cb.Report()  callbacks")
	}
}

func (cb *callbacks) OnStreamOpen(ctx context.Context, id int64, typ string) error {
	metrics.GrpcOnStreamOpen.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamOpen %d open for %s", id, typ)
	}

	return nil
}

func (cb *callbacks) OnStreamClosed(id int64) {
	metrics.GrpcOnStreamClosed.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamClosed %d closed", id)
	}
}

func (cb *callbacks) OnStreamRequest(id int64, r *discovery.DiscoveryRequest) error {
	metrics.GrpcOnStreamRequest.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamRequest")
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++

	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *callbacks) OnStreamResponse(id int64, r *discovery.DiscoveryRequest, w *discovery.DiscoveryResponse) {
	metrics.GrpcOnStreamResponse.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		json, _ := protojson.Marshal(r)
		log.Debugf("DiscoveryRequest=>\n%s\n", string(json))

		json, _ = protojson.Marshal(w)
		log.Debugf("DiscoveryResponse=>\n%s\n", string(json))
	}

	cb.Report()
}

func (cb *callbacks) OnFetchRequest(ctx context.Context, req *discovery.DiscoveryRequest) error {
	metrics.GrpcOnFetchRequest.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnFetchRequest...")
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++

	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *callbacks) OnFetchResponse(r *discovery.DiscoveryRequest, w *discovery.DiscoveryResponse) {
	metrics.GrpcOnFetchResponse.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		json, _ := protojson.Marshal(r)
		log.Debugf("DiscoveryRequest=>\n%s\n", string(json))

		json, _ = protojson.Marshal(w)
		log.Debugf("DiscoveryResponse=>\n%s\n", string(json))
	}
}

func (cb *callbacks) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	metrics.GrpcOnStreamDeltaRequest.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log := log.WithField("streamID", streamID)

		json, _ := protojson.Marshal(req)
		log.Debugf("DeltaDiscoveryRequest=>\n%s\n", string(json))
	}

	return nil
}

func (cb *callbacks) OnStreamDeltaResponse(streamID int64, req *discovery.DeltaDiscoveryRequest, resp *discovery.DeltaDiscoveryResponse) { //nolint:lll
	metrics.GrpcOnStreamDeltaResponse.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log := log.WithField("streamID", streamID)

		json, _ := protojson.Marshal(req)
		log.Debugf("DeltaDiscoveryRequest=>\n%s\n", string(json))

		json, _ = protojson.Marshal(resp)
		log.Debugf("DeltaDiscoveryResponse=>\n%s\n", string(json))
	}
}

func (cb *callbacks) OnStreamDeltaRequestOnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error { //nolint:lll,unparam
	metrics.GrpcOnStreamDeltaRequestOnStreamDeltaRequest.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log := log.WithField("streamID", streamID)

		json, _ := protojson.Marshal(req)
		log.Debugf("DeltaDiscoveryRequest=>\n%s\n", string(json))
	}

	return nil
}

func (cb *callbacks) OnDeltaStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	metrics.GrpcOnDeltaStreamOpen.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log := log.WithField("streamID", streamID)

		log.Debugf("typeURL=>\n%s\n", typeURL)
	}

	return nil
}

func (cb *callbacks) OnDeltaStreamClosed(streamID int64) {
	metrics.GrpcOnDeltaStreamClosed.Inc()

	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log := log.WithField("streamID", streamID)

		log.Debugf("closed")
	}
}

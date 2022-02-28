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
package controlplane

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
)

const filePerm = 0o644

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mutex    sync.Mutex
}

func (cb *callbacks) Report() {
	if *config.Get().LogAccess {
		log.WithFields(log.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("Report")
	}
}

func (cb *callbacks) OnStreamOpen(ctx context.Context, streamID int64, typ string) error {
	metrics.GrpcOnStreamOpen.Inc()

	if *config.Get().LogAccess {
		log.WithField("streamID", streamID).Infof("OnStreamOpen==>%s", typ)
	}

	return nil
}

func (cb *callbacks) OnStreamClosed(streamID int64) {
	metrics.GrpcOnStreamClosed.Inc()

	if *config.Get().LogAccess {
		log.WithField("streamID", streamID).Info("OnStreamClosed")
	}
}

func (cb *callbacks) OnStreamRequest(streamID int64, r *discovery.DiscoveryRequest) error {
	metrics.GrpcOnStreamRequest.Inc()

	if *config.Get().LogAccess {
		log.WithField("streamID", streamID).Info("OnStreamRequest")
	}

	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.requests++

	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *callbacks) OnStreamResponse(ctx context.Context, streamID int64, r *discovery.DiscoveryRequest, w *discovery.DiscoveryResponse) { //nolint:lll
	metrics.GrpcOnStreamResponse.Inc()

	if *config.Get().LogAccess {
		discoveryRequest, _ := protojson.Marshal(r)
		discoveryResponse, _ := protojson.Marshal(w)

		fileNameResponce := path.Join(*config.Get().LogPath, fmt.Sprintf("OnStreamResponse.%s.log", r.Node.Id))

		fResponce, err := os.OpenFile(fileNameResponce, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
		if err != nil {
			log.Println(err)
		}

		defer fResponce.Close()

		fileNameRequest := path.Join(*config.Get().LogPath, fmt.Sprintf("OnStreamRequest.%s.log", r.Node.Id))

		fRequest, err := os.OpenFile(fileNameRequest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
		if err != nil {
			log.Println(err)
		}

		defer fResponce.Close()

		discoveryRequestText := fmt.Sprintf("\nDiscoveryRequest=>\n\n%s\n\n", string(discoveryRequest))
		discoveryResponseText := fmt.Sprintf("\nDiscoveryResponse=>\n\n%s\n\n", string(discoveryResponse))

		if _, err := fRequest.WriteString(discoveryRequestText); err != nil {
			log.Println(err)
		}

		if _, err := fResponce.WriteString(discoveryResponseText); err != nil {
			log.Println(err)
		}
	}

	cb.Report()
}

func (cb *callbacks) OnFetchRequest(ctx context.Context, req *discovery.DiscoveryRequest) error {
	metrics.GrpcOnFetchRequest.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("node", req.Node.Id)

		log.Info("OnFetchRequest")
	}

	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.fetches++

	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *callbacks) OnFetchResponse(r *discovery.DiscoveryRequest, w *discovery.DiscoveryResponse) {
	metrics.GrpcOnFetchResponse.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("node", r.Node.Id)

		discoveryRequest, _ := protojson.Marshal(r)
		discoveryResponse, _ := protojson.Marshal(w)

		log.Infof("DiscoveryRequest=>%s\nDiscoveryResponse=>%s\n", string(discoveryRequest), string(discoveryResponse)) //nolint:lll
	}
}

func (cb *callbacks) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	metrics.GrpcOnStreamDeltaRequest.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("streamID", streamID)

		json, _ := protojson.Marshal(req)
		log.Infof("DeltaDiscoveryRequest=>\n%s\n", string(json))
	}

	return nil
}

func (cb *callbacks) OnStreamDeltaResponse(streamID int64, req *discovery.DeltaDiscoveryRequest, resp *discovery.DeltaDiscoveryResponse) { //nolint:lll
	metrics.GrpcOnStreamDeltaResponse.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("streamID", streamID)

		deltaDiscoveryRequest, _ := protojson.Marshal(req)
		deltaDiscoveryResponse, _ := protojson.Marshal(resp)

		log.Infof("DeltaDiscoveryRequest=>%s\nDeltaDiscoveryResponse=>%s\n", string(deltaDiscoveryRequest), string(deltaDiscoveryResponse)) //nolint:lll
	}
}

func (cb *callbacks) OnStreamDeltaRequestOnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error { //nolint: lll
	metrics.GrpcOnStreamDeltaRequestOnStreamDeltaRequest.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("streamID", streamID)

		json, _ := protojson.Marshal(req)
		log.Infof("DeltaDiscoveryRequest=>\n%s\n", string(json))
	}

	return nil
}

func (cb *callbacks) OnDeltaStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	metrics.GrpcOnDeltaStreamOpen.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("streamID", streamID)

		log.Infof("typeURL=>\n%s\n", typeURL)
	}

	return nil
}

func (cb *callbacks) OnDeltaStreamClosed(streamID int64) {
	metrics.GrpcOnDeltaStreamClosed.Inc()

	if *config.Get().LogAccess {
		log := log.WithField("streamID", streamID)

		log.Infof("OnDeltaStreamClosed")
	}
}

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
	log "github.com/sirupsen/logrus"
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
	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamOpen %d open for %s", id, typ)
	}

	return nil
}

func (cb *callbacks) OnStreamClosed(id int64) {
	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamClosed %d closed", id)
	}
}

func (cb *callbacks) OnStreamRequest(id int64, r *discovery.DiscoveryRequest) error {
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
	if log.GetLevel() >= log.DebugLevel || *appConfig.LogAccess {
		log.Debugf("OnStreamResponse...")
	}

	cb.Report()
}

func (cb *callbacks) OnFetchRequest(ctx context.Context, req *discovery.DiscoveryRequest) error {
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
func (cb *callbacks) OnFetchResponse(*discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {}

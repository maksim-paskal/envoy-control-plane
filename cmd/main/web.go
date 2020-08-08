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
	"encoding/json"
	"net/http"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	log "github.com/sirupsen/logrus"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	action := r.URL.Query()["action"]

	if len(action) < 1 {
		http.Error(w, "no action", http.StatusInternalServerError)
		return
	}

	switch action[0] {
	case "status":
		type StatusResponce struct {
			Status     []string
			StatusInfo cache.Snapshot
		}

		sn, _ := snapshotCache.GetSnapshot("test-id")

		b, _ := json.Marshal(StatusResponce{
			Status:     snapshotCache.GetStatusKeys(),
			StatusInfo: sn,
		})
		_, err := w.Write(b)
		if err != nil {
			log.Error(err)
		}

	default:
		http.Error(w, "no action defined", http.StatusInternalServerError)
	}
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type WebServer struct {
	clientset   *kubernetes.Clientset
	configStore map[string]*ConfigStore
}

func newWebServer(clientset *kubernetes.Clientset, configStore map[string]*ConfigStore) *WebServer {
	ws := WebServer{
		clientset:   clientset,
		configStore: configStore,
	}

	go func() {
		http.HandleFunc("/api/ready", ws.handlerReady)
		http.HandleFunc("/api/healthz", ws.handlerHealthz)
		http.HandleFunc("/api/status", ws.handlerStatus)
		http.HandleFunc("/api/dump_configs", ws.handlerDumpConfigs)
		http.HandleFunc("/api/zone", ws.handlerZone)
		log.Info("http.port=", *appConfig.WebAddress)
		if err := http.ListenAndServe(*appConfig.WebAddress, nil); err != nil {
			log.Fatal(err)
		}
	}()

	return &ws
}
func (ws *WebServer) handlerReady(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("ready"))
	if err != nil {
		log.Error(err)
	}
}
func (ws *WebServer) handlerHealthz(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("LIVE"))
	if err != nil {
		log.Error(err)
	}
}
func (ws *WebServer) handlerDumpConfigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var results []*ConfigType

	for _, v := range ws.configStore {
		results = append(results, v.config)
	}

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)
		return
	}

	b, _ := json.MarshalIndent(results, "", " ")
	_, err := w.Write(b)
	if err != nil {
		log.Error(err)
	}
}
func (ws *WebServer) handlerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type StatusResponce struct {
		Status     []string
		StatusInfo cache.Snapshot
	}

	var results []StatusResponce

	for _, v := range snapshotCache.GetStatusKeys() {
		sn, _ := snapshotCache.GetSnapshot(v)

		results = append(results, StatusResponce{
			Status:     snapshotCache.GetStatusKeys(),
			StatusInfo: sn,
		})
	}

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)
		return
	}

	b, _ := json.MarshalIndent(results, "", " ")
	_, err := w.Write(b)
	if err != nil {
		log.Error(err)
	}
}

func (ws *WebServer) handlerZone(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(err)
		return
	}
	namespace := r.Form.Get("namespace")
	pod := r.Form.Get("pod")

	podInfo, err := ws.clientset.CoreV1().Pods(namespace).Get(pod, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(err)
		return
	}

	nodeInfo, err := ws.clientset.CoreV1().Nodes().Get(podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(err)
		return
	}

	zone := nodeInfo.Labels[*appConfig.NodeZoneLabel]
	if len(zone) == 0 {
		zone = "unknown"
	}

	_, err = w.Write([]byte(zone))
	if err != nil {
		log.Error(err)
	}
}

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
	"fmt"
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
		http.HandleFunc("/api/config_dump", ws.handlerConfigDump)
		http.HandleFunc("/api/config_endpoints", ws.handlerConfigEndpoints)
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

func (ws *WebServer) handlerConfigDump(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	results := []*ConfigType{}

	for _, v := range ws.configStore {
		results = append(results, v.config)
	}

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		log.Error(err)
	}
	_, err = w.Write(b)
	if err != nil {
		log.Error(err)
	}
}

func (ws *WebServer) handlerConfigEndpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type EndpointsResults struct {
		Name    string
		PodInfo []string
	}
	results := []EndpointsResults{}

	for _, v := range ws.configStore {
		endpoints := EndpointsResults{
			Name: v.config.ID,
		}
		v.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
			podInfo := value.(checkPodResult)

			endpoints.PodInfo = append(endpoints.PodInfo, fmt.Sprintf("%+v", podInfo))

			return true
		})
		results = append(results, endpoints)
	}

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		log.Error(err)
	}
	_, err = w.Write(b)
	if err != nil {
		log.Error(err)
	}
}

func (ws *WebServer) handlerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type StatusResponce struct {
		NodeID   string
		Snapshot cache.Snapshot
	}

	statusKeys := snapshotCache.GetStatusKeys()

	results := []StatusResponce{}

	for _, nodeID := range statusKeys {
		sn, err := snapshotCache.GetSnapshot(nodeID)
		if err != nil {
			log.Error(err)
		}

		results = append(results, StatusResponce{
			NodeID:   nodeID,
			Snapshot: sn,
		})
	}

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		log.Error(err)
	}
	_, err = w.Write(b)
	if err != nil {
		log.Error(err)
	}
}

func (ws *WebServer) getZone(namespace string, pod string) string {
	const unknown = "unknown"

	podInfo, err := ws.clientset.CoreV1().Pods(namespace).Get(pod, metav1.GetOptions{})
	if err != nil {
		log.Error(err)

		return unknown
	}

	nodeInfo, err := ws.clientset.CoreV1().Nodes().Get(podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		log.Error(err)

		return unknown
	}

	zone := nodeInfo.Labels[*appConfig.NodeZoneLabel]

	if len(zone) == 0 {
		return unknown
	}

	return zone
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

	zone := ws.getZone(namespace, pod)

	_, err = w.Write([]byte(zone))
	if err != nil {
		log.Error(err)
	}
}

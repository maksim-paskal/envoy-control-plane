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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"

	_ "net/http/pprof"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type WebRoutes struct {
	path        string
	description string
	handler     func(w http.ResponseWriter, r *http.Request)
}

type WebServer struct {
	clientset   *kubernetes.Clientset
	configStore map[string]*ConfigStore
	routes      []WebRoutes
	log         *log.Entry
}

func newWebServer(clientset *kubernetes.Clientset, configStore map[string]*ConfigStore) *WebServer {
	ws := WebServer{
		clientset:   clientset,
		configStore: configStore,
		log: log.WithFields(log.Fields{
			"type": "ConfigStore",
		}),
	}
	ws.routes = make([]WebRoutes, 0)
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/help",
		description: "Help",
		handler:     ws.handlerHelp,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/ready",
		description: "Rediness probe",
		handler:     ws.handlerReady,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/healthz",
		description: "Health probe",
		handler:     ws.handlerHealthz,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/status",
		description: "Status",
		handler:     ws.handlerStatus,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/config_dump",
		description: "Config Dump",
		handler:     ws.handlerConfigDump,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/config_endpoints",
		description: "Config Endpoints",
		handler:     ws.handlerConfigEndpoints,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/zone",
		description: "Zone",
		handler:     ws.handlerZone,
	})
	ws.routes = append(ws.routes, WebRoutes{
		path:        "/api/version",
		description: "Get version",
		handler:     ws.handlerVersion,
	})

	go func() {
		for _, route := range ws.routes {
			http.HandleFunc(route.path, route.handler)
		}

		ws.log.Info("http.port=", *appConfig.WebAddress)

		if err := http.ListenAndServe(*appConfig.WebAddress, nil); err != nil {
			log.Fatal(err)
		}
	}()

	return &ws
}

func (ws *WebServer) handlerHelp(w http.ResponseWriter, r *http.Request) {
	var result bytes.Buffer

	linkFormat := "<div style=\"padding:5x\"><a href=\"%s\">%s</a></div><br/>"

	for _, route := range ws.routes {
		result.WriteString(fmt.Sprintf(linkFormat, route.path, route.description))
	}

	_, err := w.Write(result.Bytes())
	if err != nil {
		ws.log.Error(err)
	}
}

func (ws *WebServer) handlerReady(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("ready"))
	if err != nil {
		ws.log.Error(err)
	}
}

func (ws *WebServer) handlerHealthz(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("LIVE"))
	if err != nil {
		ws.log.Error(err)
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
		ws.log.Error(err)
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.Error(err)
	}
}

func (ws *WebServer) handlerConfigEndpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type EndpointsResults struct {
		Name      string
		Version   string
		PodInfo   []string
		LastSaved []string
	}

	results := []EndpointsResults{}

	for _, v := range ws.configStore {
		endpoints := EndpointsResults{
			Name:      v.config.ID,
			Version:   v.version,
			LastSaved: v.lastEndpointsArray,
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
		ws.log.Error(err)
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.Error(err)
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
			ws.log.Error(err)
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
		ws.log.Error(err)
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.Error(err)
	}
}

func (ws *WebServer) getZone(namespace string, pod string) string {
	const unknown = "unknown"

	podInfo, err := ws.clientset.CoreV1().Pods(namespace).Get(context.TODO(), pod, metav1.GetOptions{})
	if err != nil {
		ws.log.Error(err)

		return unknown
	}

	nodeInfo, err := ws.clientset.CoreV1().Nodes().Get(context.TODO(), podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		ws.log.Error(err)

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
		ws.log.Error(err)

		return
	}

	namespace := r.Form.Get("namespace")
	pod := r.Form.Get("pod")

	zone := ws.getZone(namespace, pod)

	_, err = w.Write([]byte(zone))
	if err != nil {
		ws.log.Error(err)
	}
}

type APIVersion struct {
	Version    string
	GoVersion  string
	Goroutines int
	GOMAXPROCS int
	GOGC       string
	GODEBUG    string
}

func (ws *WebServer) handlerVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result := APIVersion{
		Version:    appConfig.Version,
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		GOMAXPROCS: runtime.GOMAXPROCS(-1),
		GOGC:       os.Getenv("GOGC"),
		GODEBUG:    os.Getenv("GODEBUG"),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		ws.log.Error(err)
	}

	_, err = w.Write(resultJSON)

	if err != nil {
		ws.log.Error(err)
	}
}

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

	// nolint:gosec
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
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
	configStore *sync.Map
	routes      []WebRoutes
	log         *log.Entry
}

func newWebServer(clientset *kubernetes.Clientset, configStore *sync.Map) *WebServer {
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
			log.WithError(err).Fatal()
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

	if _, err := w.Write(result.Bytes()); err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *WebServer) handlerReady(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("ready"))
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *WebServer) handlerHealthz(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("LIVE")); err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *WebServer) handlerConfigDump(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	results := []*ConfigType{}

	ws.configStore.Range(func(k, v interface{}) bool {
		cs, ok := v.(*ConfigStore)

		if !ok {
			ws.log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		results = append(results, cs.config)

		return true
	})

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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

	ws.configStore.Range(func(k, v interface{}) bool {
		cs, ok := v.(*ConfigStore)

		if !ok {
			ws.log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		endpoints := EndpointsResults{
			Name:      cs.config.ID,
			Version:   cs.version,
			LastSaved: cs.lastEndpointsArray,
		}

		cs.kubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
			podInfo, ok := value.(checkPodResult)
			if !ok {
				ws.log.WithError(errAssertion).Fatal("value.(checkPodResult)")
			}

			endpoints.PodInfo = append(endpoints.PodInfo, fmt.Sprintf("%+v", podInfo))

			return true
		})

		results = append(results, endpoints)

		return true
	})

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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
			ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *WebServer) getZone(namespace string, pod string) string {
	const unknown = "unknown"

	podInfo, err := ws.clientset.CoreV1().Pods(namespace).Get(context.TODO(), pod, metav1.GetOptions{})
	if err != nil {
		ws.log.WithError(err).Error()

		return unknown
	}

	nodeInfo, err := ws.clientset.CoreV1().Nodes().Get(context.TODO(), podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		ws.log.WithError(err).Error()

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
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()

		return
	}

	namespace := r.Form.Get("namespace")
	pod := r.Form.Get("pod")

	zone := ws.getZone(namespace, pod)

	_, err = w.Write([]byte(zone))
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(resultJSON)

	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

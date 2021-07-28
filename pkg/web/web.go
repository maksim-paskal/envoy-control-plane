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
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Routes struct {
	path        string
	description string
	handler     func(w http.ResponseWriter, r *http.Request)
}

type Server struct {
	routes []Routes
	log    *log.Entry
	ctx    context.Context
}

func NewServer() *Server {
	ws := Server{
		ctx: context.Background(),
		log: log.WithFields(log.Fields{
			"type": "ConfigStore",
		}),
	}
	ws.routes = make([]Routes, 0)
	ws.routes = append(ws.routes, Routes{
		path:        "/api/help",
		description: "Help",
		handler:     ws.handlerHelp,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/ready",
		description: "Rediness probe",
		handler:     ws.handlerReady,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/healthz",
		description: "Health probe",
		handler:     ws.handlerHealthz,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/status",
		description: "Status",
		handler:     ws.handlerStatus,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/config_dump",
		description: "Config Dump",
		handler:     ws.handlerConfigDump,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/config_endpoints",
		description: "Config Endpoints",
		handler:     ws.handlerConfigEndpoints,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/zone",
		description: "Zone",
		handler:     ws.handlerZone,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/version",
		description: "Get version",
		handler:     ws.handlerVersion,
	})

	return &ws
}

func (ws *Server) Start() {
	ws.log.Info("http.port=", *config.Get().WebAddress)

	if err := http.ListenAndServe(*config.Get().WebAddress, ws.GetHandler()); err != nil {
		log.WithError(err).Fatal()
	}
}

func (ws *Server) GetHandler() *http.ServeMux {
	mux := http.NewServeMux()

	for _, route := range ws.routes {
		mux.HandleFunc(route.path, route.handler)
	}

	mux.Handle("/api/metrics", metrics.GetHandler())

	// pprof
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}

func (ws *Server) handlerHelp(w http.ResponseWriter, r *http.Request) {
	var result bytes.Buffer

	linkFormat := "<div style=\"padding:5x\"><a href=\"%s\">%s</a></div><br/>"

	for _, route := range ws.routes {
		result.WriteString(fmt.Sprintf(linkFormat, route.path, route.description))
	}

	if _, err := w.Write(result.Bytes()); err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *Server) handlerReady(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("ready"))
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *Server) handlerHealthz(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("LIVE")); err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func (ws *Server) handlerConfigDump(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	results := []*config.ConfigType{}

	configstore.StoreMap.Range(func(k, v interface{}) bool {
		cs, ok := v.(*configstore.ConfigStore)

		if !ok {
			ws.log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		results = append(results, cs.Config)

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

func (ws *Server) handlerConfigEndpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type EndpointsResults struct {
		Name      string
		Version   string
		PodInfo   []string
		LastSaved []string
	}

	results := []EndpointsResults{}

	configstore.StoreMap.Range(func(k, v interface{}) bool {
		cs, ok := v.(*configstore.ConfigStore)

		if !ok {
			ws.log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		endpoints := EndpointsResults{
			Name:      cs.Config.ID,
			Version:   cs.Version,
			LastSaved: cs.LastEndpointsArray,
		}

		cs.KubernetesEndpoints.Range(func(key interface{}, value interface{}) bool {
			podInfo, ok := value.(configstore.CheckPodResult)
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

func (ws *Server) handlerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type StatusResponce struct {
		NodeID   string
		Snapshot cache.Snapshot
	}

	statusKeys := controlplane.SnapshotCache.GetStatusKeys()

	results := []StatusResponce{}

	for _, nodeID := range statusKeys {
		sn, err := controlplane.SnapshotCache.GetSnapshot(nodeID)
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

func (ws *Server) getZone(namespace string, pod string) string {
	const unknown = "unknown"

	podInfo, err := api.Clientset.CoreV1().Pods(namespace).Get(ws.ctx, pod, metav1.GetOptions{})
	if err != nil {
		ws.log.WithError(err).Error()

		return unknown
	}

	nodeInfo, err := api.Clientset.CoreV1().Nodes().Get(ws.ctx, podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		ws.log.WithError(err).Error()

		return unknown
	}

	zone := nodeInfo.Labels[*config.Get().NodeZoneLabel]

	if len(zone) == 0 {
		return unknown
	}

	return zone
}

func (ws *Server) handlerZone(w http.ResponseWriter, r *http.Request) {
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

func (ws *Server) handlerVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result := APIVersion{
		Version:    config.GetVersion(),
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

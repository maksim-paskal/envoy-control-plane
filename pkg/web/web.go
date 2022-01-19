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
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	basicRealm  = config.AppName
	adminPrefix = "/api/admin"
)

type Routes struct {
	path        string
	description string
	handlerFunc func(w http.ResponseWriter, r *http.Request)
	handler     http.Handler
	httpShema   bool
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
		handlerFunc: ws.handlerHelp,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/ready",
		description: "Route for application rediness probe",
		handlerFunc: ws.handlerReady,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/healthz",
		description: "Route for application health probe",
		handlerFunc: ws.handlerHealthz,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/admin/status",
		description: "Status all nodes in SnapshotCache ",
		handlerFunc: ws.handlerStatus,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/admin/config_dump",
		description: "All dumps of configs that loaded to control-plane",
		handlerFunc: ws.handlerConfigDump,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/config_endpoints",
		description: "All endpoints in configs",
		handlerFunc: ws.handlerConfigEndpoints,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/zone",
		description: "Get pod zone",
		handlerFunc: ws.handlerZone,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/version",
		description: "Get version",
		handlerFunc: ws.handlerVersion,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/metrics",
		httpShema:   true,
		description: "Get metrics",
		handler:     metrics.GetHandler(),
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/api/admin/certs",
		description: "Generate cert",
		handlerFunc: ws.handlerCerts,
	})

	// pprof
	ws.routes = append(ws.routes, Routes{
		path:        "/debug/pprof/",
		handlerFunc: pprof.Index,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/debug/pprof/cmdline",
		handlerFunc: pprof.Cmdline,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/debug/pprof/profile",
		handlerFunc: pprof.Profile,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/debug/pprof/symbol",
		handlerFunc: pprof.Symbol,
	})
	ws.routes = append(ws.routes, Routes{
		path:        "/debug/pprof/trace",
		handlerFunc: pprof.Trace,
	})

	return &ws
}

func (ws *Server) Start() {
	ws.log.Info("http.address=", *config.Get().WebHTTPAddress)

	if err := http.ListenAndServe(*config.Get().WebHTTPAddress, auth(ws.GetHandler(true))); err != nil {
		log.WithError(err).Fatal()
	}
}

func (ws *Server) StartTLS() {
	ws.log.Info("https.address=", *config.Get().WebHTTPSAddress)

	_, serverCertBytes, _, serverKeyBytes, err := certs.NewCertificate(config.AppName, certs.CertValidityYear)
	if err != nil {
		log.WithError(err).Fatal("failed to NewCertificate")
	}

	cert, err := tls.X509KeyPair(serverCertBytes, serverKeyBytes)
	if err != nil {
		log.WithError(err).Fatal("failed to NewCertificate")
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	server := http.Server{
		Addr:      *config.Get().WebHTTPSAddress,
		TLSConfig: tlsConfig,
		Handler:   auth(ws.GetHandler(false)),
	}

	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.WithError(err).Fatal()
	}
}

func (ws *Server) GetHandler(onlyHTTPShema bool) *http.ServeMux {
	mux := http.NewServeMux()

	for _, route := range ws.routes {
		// add only routes with httpShema if onlyHttpShema
		if !onlyHTTPShema || route.httpShema == onlyHTTPShema {
			if route.handler != nil {
				mux.Handle(route.path, route.handler)
			} else {
				mux.HandleFunc(route.path, route.handlerFunc)
			}
		}
	}

	return mux
}

func auth(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, adminPrefix) {
			user, pass, ok := r.BasicAuth()

			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(*config.Get().WebAdminUser)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(*config.Get().WebAdminPassword)) != 1 { //nolint:lll
				w.Header().Set("WWW-Authenticate", `Basic realm="`+basicRealm+`"`)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorised"))

				return
			}
		}

		log := log.WithFields(
			log.Fields{
				"RemoteAddr": r.RemoteAddr,
				"Method":     r.Method,
			},
		)

		if r.URL.Path == "/api/ready" || r.URL.Path == "/api/healthz" || r.URL.Path == "/api/metrics" {
			log.Debug(r.URL)
		} else {
			log.Info(r.URL)
		}

		mux.ServeHTTP(w, r)
	})
}

func (ws *Server) handlerHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	var result bytes.Buffer

	linkFormat := "<tr><td>%s</td><td><a href='%s'>%s</a></td></tr>"

	result.WriteString("<table border='1' cellpadding='8'>")

	for _, route := range ws.routes {
		if len(route.description) > 0 {
			result.WriteString(fmt.Sprintf(linkFormat, route.description, route.path, route.path))
		}
	}

	result.WriteString("</table>")

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

	_ = r.ParseForm()

	id := r.Form.Get("id")

	configstore.StoreMap.Range(func(k, v interface{}) bool {
		cs, ok := v.(*configstore.ConfigStore)

		if !ok {
			ws.log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		if len(id) == 0 || cs.Config.ID == id {
			results = append(results, cs.Config)
		}

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

	_ = r.ParseForm()

	id := r.Form.Get("id")

	for _, nodeID := range statusKeys {
		sn, err := controlplane.SnapshotCache.GetSnapshot(nodeID)
		if err != nil {
			ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
		}

		if len(id) == 0 || id == nodeID {
			results = append(results, StatusResponce{
				NodeID:   nodeID,
				Snapshot: sn,
			})
		}
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

func (ws *Server) handlerZone(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()

		return
	}

	namespace := r.Form.Get("namespace")
	pod := r.Form.Get("pod")

	zone := api.GetZone(namespace, pod)

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

func (ws *Server) handlerCerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	if err := r.ParseForm(); err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	certDuration := certs.CertValidity

	name := r.Form.Get("name")
	duration := r.Form.Get("duration")

	if len(name) == 0 {
		http.Error(w, "no name", http.StatusBadRequest)

		return
	}

	if len(duration) == 0 {
		http.Error(w, "no duration", http.StatusBadRequest)

		return
	}

	if len(duration) > 0 {
		parsedDuration, err := time.ParseDuration(duration)
		if err != nil {
			http.Error(w, errors.Wrap(err, "error in parsing duration").Error(), http.StatusBadRequest)

			return
		}

		certDuration = parsedDuration
	}

	_, crtBytes, _, keyBytes, err := certs.NewCertificate(name, certDuration)
	if err != nil {
		ws.log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, _ = w.Write(crtBytes)
	_, _ = w.Write(keyBytes)
}

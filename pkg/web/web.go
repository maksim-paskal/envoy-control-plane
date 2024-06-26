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
	basicRealm           = config.AppName
	adminPrefix          = "/api/admin"
	serverReadTimeout    = 5 * time.Second
	serverRequestTimeout = 5 * time.Second
	serverWriteTimeout   = 10 * time.Second
)

var timeoutMessage = fmt.Sprintf("Server timeout after %s", serverRequestTimeout)

type Route struct {
	path        string
	description string
	handlerFunc func(w http.ResponseWriter, r *http.Request)
	handler     http.Handler
	httpShema   bool
}

func getRoutes() []Route {
	routes := make([]Route, 0)

	routes = append(routes, Route{
		path:        "/api/help",
		description: "Help",
		handlerFunc: handlerHelp,
	})
	routes = append(routes, Route{
		path:        "/api/ready",
		description: "Route for application rediness probe",
		handlerFunc: handlerReady,
	})
	routes = append(routes, Route{
		path:        "/api/healthz",
		description: "Route for application health probe",
		handlerFunc: handlerHealthz,
	})
	routes = append(routes, Route{
		path:        "/api/admin/status",
		description: "Status all nodes in SnapshotCache ",
		handlerFunc: handlerStatus,
	})
	routes = append(routes, Route{
		path:        "/api/admin/config_dump",
		description: "All dumps of configs that loaded to control-plane",
		handlerFunc: handlerConfigDump,
	})
	routes = append(routes, Route{
		path:        "/api/config_endpoints",
		description: "All endpoints in configs",
		handlerFunc: handlerConfigEndpoints,
	})
	routes = append(routes, Route{
		path:        "/api/zone",
		description: "Get pod zone",
		handlerFunc: handlerZone,
	})
	routes = append(routes, Route{
		path:        "/api/version",
		description: "Get version",
		handlerFunc: handlerVersion,
	})
	routes = append(routes, Route{
		path:        "/api/metrics",
		httpShema:   true,
		description: "Get metrics",
		handler:     metrics.GetHandler(),
	})
	routes = append(routes, Route{
		path:        "/api/admin/certs",
		description: "Generate cert",
		handlerFunc: handlerCerts,
	})

	// pprof
	routes = append(routes, Route{
		path:        "/debug/pprof/",
		handlerFunc: pprof.Index,
	})
	routes = append(routes, Route{
		path:        "/debug/pprof/cmdline",
		handlerFunc: pprof.Cmdline,
	})
	routes = append(routes, Route{
		path:        "/debug/pprof/profile",
		handlerFunc: pprof.Profile,
	})
	routes = append(routes, Route{
		path:        "/debug/pprof/symbol",
		handlerFunc: pprof.Symbol,
	})
	routes = append(routes, Route{
		path:        "/debug/pprof/trace",
		handlerFunc: pprof.Trace,
	})

	return routes
}

func Start(ctx context.Context) {
	log.Info("http.address=", *config.Get().WebHTTPAddress)

	server := &http.Server{
		Addr:         *config.Get().WebHTTPAddress,
		Handler:      http.TimeoutHandler(auth(GetHandler(true)), serverRequestTimeout, timeoutMessage),
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
	}

	go func() {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), *config.Get().GracePeriod)
		defer cancel()

		_ = server.Shutdown(ctx) //nolint:contextcheck
	}()

	// start http server
	if err := server.ListenAndServe(); err != nil {
		log.WithError(err).Fatal()
	}
}

func StartTLS(ctx context.Context) {
	log.Info("https.address=", *config.Get().WebHTTPSAddress)

	_, serverCertBytes, _, serverKeyBytes, err := certs.NewCertificate([]string{config.AppName}, certs.CertValidityYear)
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
		Addr:         *config.Get().WebHTTPSAddress,
		TLSConfig:    tlsConfig,
		Handler:      http.TimeoutHandler(auth(GetHandler(false)), serverRequestTimeout, timeoutMessage),
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
	}

	go func() {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), *config.Get().GracePeriod)
		defer cancel()

		_ = server.Shutdown(ctx) //nolint:contextcheck
	}()

	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.WithError(err).Fatal()
	}
}

func GetHandler(onlyHTTPShema bool) *http.ServeMux {
	mux := http.NewServeMux()

	for _, route := range getRoutes() {
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

func handlerHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	var result bytes.Buffer

	linkFormat := "<tr><td>%s</td><td><a href='%s'>%s</a></td></tr>"

	result.WriteString("<table border='1' cellpadding='8'>")

	for _, route := range getRoutes() {
		if len(route.description) > 0 {
			result.WriteString(fmt.Sprintf(linkFormat, route.description, route.path, route.path))
		}
	}

	result.WriteString("</table>")

	if _, err := w.Write(result.Bytes()); err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerReady(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("ready"))
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerHealthz(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("LIVE")); err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerConfigDump(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	results := []*config.ConfigType{}

	_ = r.ParseForm()

	id := r.Form.Get("id")

	configstore.StoreMap.Range(func(_, v interface{}) bool {
		cs, ok := v.(*configstore.ConfigStore)

		if !ok {
			log.WithError(errAssertion).Fatal("handlerConfigDump v.(*ConfigStore)")
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
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerConfigEndpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	_ = r.ParseForm()

	id := r.Form.Get("id")

	type EndpointsResults struct {
		Name      string
		Version   string
		LastSaved []string
	}

	results := []EndpointsResults{}

	configstore.StoreMap.Range(func(_, v interface{}) bool {
		cs, ok := v.(*configstore.ConfigStore)

		if !ok {
			log.WithError(errAssertion).Fatal("handlerConfigEndpoints v.(*ConfigStore)")
		}

		endpoints := EndpointsResults{
			Name:      cs.Config.ID,
			Version:   cs.Version,
			LastSaved: cs.GetLastEndpoints(),
		}

		if len(id) == 0 || cs.Config.ID == id {
			results = append(results, endpoints)
		}

		return true
	})

	if len(results) == 0 {
		http.Error(w, "no results", http.StatusInternalServerError)

		return
	}

	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type StatusResponce struct {
		NodeID   string
		Snapshot cache.ResourceSnapshot
	}

	statusKeys := controlplane.SnapshotCache.GetStatusKeys()

	results := []StatusResponce{}

	_ = r.ParseForm()

	id := r.Form.Get("id")

	for _, nodeID := range statusKeys {
		sn, err := controlplane.SnapshotCache.GetSnapshot(nodeID)
		if err != nil {
			log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(b)
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerZone(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()

		return
	}

	namespace := r.Form.Get("namespace")
	pod := r.Form.Get("pod")

	zone := api.GetZoneByPodName(r.Context(), namespace, pod)

	_, err = w.Write([]byte(zone))
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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

func handlerVersion(w http.ResponseWriter, r *http.Request) {
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
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, err = w.Write(resultJSON)
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}
}

func handlerCerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	if err := r.ParseForm(); err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
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

	_, crtBytes, _, keyBytes, err := certs.NewCertificate([]string{name}, certDuration)
	if err != nil {
		log.WithFields(logrushooksentry.AddRequest(r)).WithError(err).Error()
	}

	_, _ = w.Write(crtBytes)
	_, _ = w.Write(keyBytes)
}

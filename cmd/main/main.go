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
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	log "github.com/sirupsen/logrus"
)

var snapshotCache cache.SnapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, &Logger{})
var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

func loadConfigDirectory(filePath string) {
	_, err := os.Stat(filePath)

	if err != nil {
		log.Error(err)
		return
	}

	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		log.Error(err)
		return
	}
	for _, f := range files {
		loadFile := !f.IsDir()
		name := f.Name()

		if strings.HasPrefix(name, "_") || strings.EqualFold(name, "values.yaml") {
			loadFile = false
		}

		if loadFile {
			new := ConfigStore{}

			err := new.LoadFile(filepath.Join(filePath, f.Name()))
			if err != nil {
				log.Error(err)
			} else {
				obj := configStore[new.config.Id]
				if obj == nil {
					configStore[new.config.Id] = &new
				} else {
					if !reflect.DeepEqual(obj.config, new.config) {
						configStore[new.config.Id] = &new
					}
				}
			}
		}
	}
}
func main() {
	flag.Parse()

	logLevel, err := log.ParseLevel(*appConfig.LogLevel)
	if err != nil {
		log.Panic(err)
	}
	if *appConfig.LogInJSON {
		log.SetFormatter(&log.JSONFormatter{})
	}
	log.SetLevel(logLevel)

	log.Debugf("loaded application config = \n%s", appConfig.String())

	if *appConfig.ReadConfigDir {
		loadConfigDirectory(*appConfig.ConfigDirectory)
	}

	var kubeconfig *rest.Config

	if len(*appConfig.KubeconfigFile) > 0 {
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", *appConfig.KubeconfigFile)
		if err != nil {
			log.Panic(err)
		}
	} else {
		kubeconfig, err = rest.InClusterConfig()
		if err != nil {
			log.Panic(err)
		}
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Panic(err)
	}
	if *appConfig.ReadConfigMap {
		cmStore := newConfigMapStore(clientset)
		defer cmStore.Stop()
	}

	epStore := newEndpointsStore(clientset, nil)
	defer epStore.Stop()

	ctx := context.Background()

	signal := make(chan struct{})
	cb := &callbacks{
		signal:   signal,
		fetches:  0,
		requests: 0,
	}

	log.Info("grpcServer.port=", ":18080")
	server := xds.NewServer(ctx, snapshotCache, cb)
	grpcServer := grpc.NewServer()
	lis, _ := net.Listen("tcp", ":18080")

	als := &AccessLogService{}

	accesslog.RegisterAccessLogServiceServer(grpcServer, als)
	api.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	api.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	api.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	api.RegisterListenerDiscoveryServiceServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		http.HandleFunc("/", handler)
		log.Info("http.port=", ":18081")
		if err := http.ListenAndServe(":18081", nil); err != nil {
			log.Fatal(err)
		}
	}()

	<-signal

	cb.Report()

	<-ctx.Done()
	grpcServer.GracefulStop()
}

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
	"net"
	"net/http"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	log "github.com/sirupsen/logrus"
)

var snapshotCache cache.SnapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, &Logger{})
var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

func loadConfigDirectory() {
	_, err := os.Stat(*appConfig.Config)

	if err != nil {
		log.Panic(err)
	}

	err = filepath.Walk(*appConfig.Config, func(path string, info os.FileInfo, err error) error {

		if info.IsDir() {
			return nil
		}

		test := ConfigStore{}

		test.LoadFile(path)

		configStore[test.config.Id] = &test

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	flag.Parse()

	logLevel, err := log.ParseLevel(*appConfig.LogLevel)
	if err != nil {
		log.Panic(err)
	}
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(logLevel)

	log.Debug(appConfig.String())

	loadConfigDirectory()

	if *appConfig.WithKubernetesWatch {
		kubeconfig, err := clientcmd.BuildConfigFromFlags("", "kubeconfig")
		if err != nil {
			log.Panic(err)
		}
		clientset, err := kubernetes.NewForConfig(kubeconfig)
		if err != nil {
			log.Panic(err)
		}

		epStore := newEndpointsStore(clientset, nil)
		defer epStore.Stop()
	}

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
	//discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
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

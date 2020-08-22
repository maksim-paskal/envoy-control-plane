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

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getKubernetesClient() *kubernetes.Clientset {
	var kubeconfig *rest.Config
	var err error

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
	return clientset
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

	if logLevel == log.DebugLevel {
		log.SetReportCaller(true)
	}

	log.SetLevel(logLevel)

	log.Debugf("loaded application config = \n%s", appConfig.String())

	var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

	clientset := getKubernetesClient()

	ep := newEndpointsStore(clientset)

	ep.onNewPod = func(pod *v1.Pod) {
		for _, v := range configStore {
			v.newPod(pod)
		}
	}
	defer ep.Stop()

	cms := newConfigMapStore(clientset)

	cms.onNewConfig = func(config ConfigType) {
		if configStore[config.Id] != nil {
			configStore[config.Id].Stop()
		}

		log.Infof("Create configStore %s", config.Id)
		configStore[config.Id] = newConfigStore(config, ep)
	}

	defer cms.Stop()

	ctx := context.Background()
	var grpcOptions []grpc.ServerOption
	//grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", *appConfig.GrpcAddress)
	if err != nil {
		log.Fatal(err)
	}

	startControlPlane(ctx, grpcServer)
	newWebServer(clientset)

	log.Printf("management server listening on %s\n", *appConfig.GrpcAddress)

	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Println(err)
		}
	}()

	<-ctx.Done()
}

var snapshotCache cache.SnapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, &Logger{})

func startControlPlane(ctx context.Context, grpcServer *grpc.Server) {
	signal := make(chan struct{})
	cb := &callbacks{
		signal:   signal,
		fetches:  0,
		requests: 0,
	}

	als := &AccessLogService{}

	server := xds.NewServer(ctx, snapshotCache, cb)

	accesslog.RegisterAccessLogServiceServer(grpcServer, als)
	api.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	api.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	api.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	api.RegisterListenerDiscoveryServiceServer(grpcServer, server)
}

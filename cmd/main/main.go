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

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
)

var buildTime = "dev"

const (
	grpcMaxConcurrentStreams = 1000000
)

func main() {
	flag.Parse()

	err := appConfig.CheckConfig()
	if err != nil {
		log.Fatal(err)
	}

	logLevel, err := log.ParseLevel(*appConfig.LogLevel)
	if err != nil {
		log.Fatal(err)
	}
	if *appConfig.LogInJSON {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if logLevel == log.DebugLevel {
		log.SetReportCaller(true)
	}

	log.SetLevel(logLevel)

	log.Infof("Starting %s...", appConfig.Version)

	log.Debugf("loaded application config = \n%s", appConfig.String())

	clientset, err := getKubernetesClient()
	if err != nil {
		log.Fatal(err)
	}

	var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

	ep := newEndpointsStore(clientset)

	ep.onNewPod = func(pod *v1.Pod) {
		for _, v := range configStore {
			v.newPod(pod)
		}
	}
	defer ep.Stop()

	cms := newConfigMapStore(clientset)

	cms.onNewConfig = func(config *ConfigType) {
		if configStore[config.Id] != nil {
			configStore[config.Id].Stop()
		}

		log.Infof("Create configStore %s", config.Id)
		configStore[config.Id] = newConfigStore(config, ep)
	}

	defer cms.Stop()

	ctx := context.Background()
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)
	defer grpcServer.GracefulStop()

	lis, err := net.Listen("tcp", *appConfig.GrpcAddress)
	if err != nil {
		log.Fatal(err)
	}

	newControlPlane(ctx, grpcServer)
	newWebServer(clientset)

	log.Printf("management server listening on %s\n", *appConfig.GrpcAddress)

	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
}

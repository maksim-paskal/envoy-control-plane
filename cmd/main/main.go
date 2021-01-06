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
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	sentrylogrushook "github.com/maksim-paskal/sentry-logrus-hook"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
)

var (
	gitVersion string = "dev"
	buildTime  string
)

const (
	grpcMaxConcurrentStreams = 1000000
)

func main() {
	flag.Parse()

	if *appConfig.showVersion {
		fmt.Println(appConfig.Version)
		os.Exit(0)
	}

	if len(*appConfig.ConfigFile) > 0 {
		yamlFile, err := ioutil.ReadFile(*appConfig.ConfigFile)
		if err != nil {
			log.WithError(err).Fatal()
		}

		err = yaml.Unmarshal(yamlFile, &appConfig)
		if err != nil {
			log.WithError(err).Fatal()
		}
	}

	err := appConfig.CheckConfig()
	if err != nil {
		log.WithError(err).Fatal()
	}

	logLevel, err := log.ParseLevel(*appConfig.LogLevel)
	if err != nil {
		log.WithError(err).Fatal()
	}

	if *appConfig.LogPretty {
		log.SetFormatter(&log.TextFormatter{})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if logLevel == log.DebugLevel {
		log.SetReportCaller(true)
	}

	log.SetLevel(logLevel)

	hook, err := sentrylogrushook.NewHook(sentrylogrushook.SentryLogHookOptions{
		SentryDSN: *appConfig.SentryDSN,
		Release:   appConfig.Version,
	})
	if err != nil {
		log.WithError(err).Error()
	}

	log.AddHook(hook)

	log.Infof("Starting %s...", appConfig.Version)
	log.Debugf("loaded application config = \n%s", appConfig.String())

	clientset, err := getKubernetesClient()
	if err != nil {
		log.WithError(err).Fatal()
	}

	var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

	ep := newEndpointsStore(clientset)

	ep.onNewPod = func(pod *v1.Pod) {
		for _, v := range configStore {
			if v.ConfigStoreState != ConfigStoreStateSTOP {
				v.NewPod(pod)
			}
		}
	}

	ep.onDeletePod = func(pod *v1.Pod) {
		for _, v := range configStore {
			if v.ConfigStoreState != ConfigStoreStateSTOP {
				v.DeletePod(pod)
			}
		}
	}
	defer ep.Stop()

	cms := newConfigMapStore(clientset)

	cms.onNewConfig = func(config *ConfigType) {
		if configStore[config.ID] != nil {
			configStore[config.ID].Stop()
		}

		log.Infof("Create configStore %s", config.ID)
		configStore[config.ID] = newConfigStore(config, ep)
	}

	cms.onDeleteConfig = func(nodeID string) {
		if configStore[nodeID] != nil {
			configStore[nodeID].Stop()

			drainPeriod, err := time.ParseDuration(*appConfig.ConfigDrainPeriod)

			if err != nil {
				log.WithError(err).Error()
			} else {
				time.Sleep(drainPeriod)
			}

			delete(configStore, nodeID)
			snapshotCache.ClearSnapshot(nodeID)
		}
	}

	defer cms.Stop()

	ctx := context.Background()
	grpcOptions := []grpc.ServerOption{}
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	defer grpcServer.GracefulStop()

	lis, err := net.Listen("tcp", *appConfig.GrpcAddress)
	if err != nil {
		log.WithError(err).Error()

		return
	}

	newControlPlane(ctx, grpcServer)
	newWebServer(clientset, configStore)

	log.Infof("management server listening on %s\n", *appConfig.GrpcAddress)

	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.WithError(err).Fatal()
		}
	}()

	// sync manual
	go func() {
		WaitTime, err := time.ParseDuration(*appConfig.EndpointCheckPeriod)
		if err != nil {
			log.WithError(err).Fatal()
		}

		for {
			time.Sleep(WaitTime)

			for _, v := range configStore {
				if v.ConfigStoreState != ConfigStoreStateSTOP {
					log.Debugf("check endpoints=%s", v.config.ID)
					v.Sync()
				}
			}
		}
	}()

	defer hook.Stop()

	<-ctx.Done()
}

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
	"os"
	"sync"
	"time"

	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
)

var gitVersion = "dev"

const (
	grpcMaxConcurrentStreams = 1000000
)

func main() {
	flag.Parse()

	if *appConfig.showVersion {
		os.Stdout.WriteString(appConfig.Version)
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

	hook, err := logrushooksentry.NewHook(logrushooksentry.Options{
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

	configStore := new(sync.Map)

	ep := newEndpointsStore(clientset)

	ep.onNewPod = func(pod *v1.Pod) {
		configStore.Range(func(k, v interface{}) bool {
			cs, ok := v.(*ConfigStore)

			if !ok {
				ep.log.WithError(ErrAssertion).Fatal("v.(*ConfigStore)")
			}

			if cs.ConfigStoreState != ConfigStoreStateSTOP {
				cs.NewPod(pod)
			}

			return true
		})
	}

	ep.onDeletePod = func(pod *v1.Pod) {
		configStore.Range(func(k, v interface{}) bool {
			cs, ok := v.(*ConfigStore)

			if !ok {
				ep.log.WithError(ErrAssertion).Fatal("v.(*ConfigStore)")
			}

			if cs.ConfigStoreState != ConfigStoreStateSTOP {
				cs.DeletePod(pod)
			}

			return true
		})
	}
	defer ep.Stop()

	cms := newConfigMapStore(clientset)

	cms.onNewConfig = func(config *ConfigType) {
		// delete entry in map if exists
		if v, ok := configStore.Load(config.ID); ok {
			cs, ok := v.(*ConfigStore)

			if !ok {
				ep.log.WithError(ErrAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Stop()
		}

		log.Infof("Create configStore %s", config.ID)
		configStore.Store(config.ID, newConfigStore(config, ep))
	}

	cms.onDeleteConfig = func(nodeID string) {
		if v, ok := configStore.Load(nodeID); ok {
			cs, ok := v.(*ConfigStore)

			if !ok {
				ep.log.WithError(ErrAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Stop()

			drainPeriod, err := time.ParseDuration(*appConfig.ConfigDrainPeriod)

			if err != nil {
				log.WithError(err).Error()
			} else {
				time.Sleep(drainPeriod)
			}

			configStore.Delete(nodeID)
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

			configStore.Range(func(k, v interface{}) bool {
				cs, ok := v.(*ConfigStore)

				if !ok {
					log.WithError(ErrAssertion).Fatal("v.(*ConfigStore)")
				}

				if cs.ConfigStoreState != ConfigStoreStateSTOP {
					log.Debugf("check endpoints=%s", cs.config.ID)
					cs.Sync()
				}

				return true
			})
		}
	}()

	defer hook.Stop()

	<-ctx.Done()
}

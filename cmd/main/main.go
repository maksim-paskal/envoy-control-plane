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
	"net"
	"os"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configmapsstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/endpointstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/web"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	v1 "k8s.io/api/core/v1"
)

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

var version = flag.Bool("version", false, "version")

func main() {
	flag.Parse()

	if *version {
		fmt.Println(config.GetVersion()) //nolint:forbidigo
		os.Exit(0)
	}

	var err error

	if err = config.Load(); err != nil {
		log.Fatal(err)
	}

	logLevel, err := log.ParseLevel(*config.Get().LogLevel)
	if err != nil {
		log.WithError(err).Fatal()
	}

	log.SetLevel(logLevel)

	log.Debugf("Using config:\n%s", config.String())

	if *config.Get().LogPretty {
		log.SetFormatter(&log.TextFormatter{})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if logLevel == log.DebugLevel || *config.Get().LogReportCaller {
		log.SetReportCaller(true)
	}

	if err = config.CheckConfig(); err != nil {
		log.Fatal(err)
	}

	hook, err := logrushooksentry.NewHook(logrushooksentry.Options{
		SentryDSN: *config.Get().SentryDSN,
		Release:   config.GetVersion(),
	})
	if err != nil {
		log.WithError(err).Error()
	}

	log.AddHook(hook)

	log.Infof("Starting %s...", config.GetVersion())

	if err = api.MakeAuth(); err != nil {
		log.WithError(err).Fatal()
	}

	ep := endpointstore.New()

	ep.OnNewPod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			if cs.ConfigStoreState != configstore.ConfigStoreStateSTOP {
				cs.NewPod(pod)
			}

			return true
		})
	}

	ep.OnDeletePod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			if cs.ConfigStoreState != configstore.ConfigStoreStateSTOP {
				cs.DeletePod(pod)
			}

			return true
		})
	}
	defer ep.Stop()

	cms := configmapsstore.New()

	cms.OnNewConfig = func(config *config.ConfigType) {
		// delete entry in map if exists
		if v, ok := configstore.StoreMap.Load(config.ID); ok {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Stop()
		}

		log.Infof("Create configStore %s", config.ID)
		configstore.StoreMap.Store(config.ID, configstore.New(config, ep))
	}

	cms.OnDeleteConfig = func(nodeID string) {
		if v, ok := configstore.StoreMap.Load(nodeID); ok {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Stop()

			drainPeriod, err := time.ParseDuration(*config.Get().ConfigDrainPeriod)

			if err != nil {
				log.WithError(err).Error()
			} else {
				time.Sleep(drainPeriod)
			}

			configstore.StoreMap.Delete(nodeID)
			controlplane.SnapshotCache.ClearSnapshot(nodeID)
		}
	}

	defer cms.Stop()

	ctx := context.Background()
	grpcOptions := []grpc.ServerOption{}
	grpcOptions = append(grpcOptions,
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    grpcKeepaliveTime,
			Timeout: grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcKeepaliveMinTime,
			PermitWithoutStream: true,
		}),
	)
	grpcServer := grpc.NewServer(grpcOptions...)

	defer grpcServer.GracefulStop()

	lis, err := net.Listen("tcp", *config.Get().GrpcAddress)
	if err != nil {
		log.WithError(err).Error()

		return
	}

	controlplane.New(ctx, grpcServer)

	go web.NewServer().Start()

	log.Info("grpc.port=", *config.Get().GrpcAddress)

	go startGRPC(grpcServer, lis)

	// sync manual
	go syncManual()

	defer hook.Stop()

	<-ctx.Done()
}

// starts GRPC server.
func startGRPC(grpcServer *grpc.Server, lis net.Listener) {
	if err := grpcServer.Serve(lis); err != nil {
		log.WithError(err).Fatal()
	}
}

// sync all endpoints in configs with endpointstore.
func syncManual() {
	WaitTime, err := time.ParseDuration(*config.Get().EndpointCheckPeriod)
	if err != nil {
		log.WithError(err).Fatal()
	}

	for {
		time.Sleep(WaitTime)

		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			if cs.ConfigStoreState != configstore.ConfigStoreStateSTOP {
				log.Debugf("check endpoints=%s", cs.Config.ID)
				cs.Sync()
			}

			return true
		})
	}
}

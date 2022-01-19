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
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configmapsstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/endpointstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/web"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	if err = api.Init(); err != nil {
		log.WithError(err).Fatal()
	}

	if err = certs.Init(); err != nil {
		log.WithError(err).Fatal()
	}

	go rotateCertificates()

	ep := endpointstore.New()

	ep.OnNewPod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.NewPod(pod)

			return true
		})
	}

	ep.OnDeletePod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.DeletePod(pod)

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

		newConfigStore, err := configstore.New(config, ep)
		if err != nil {
			log.WithError(err).Error("error in creating configstore")
		}

		configstore.StoreMap.Store(config.ID, newConfigStore)
	}

	cms.OnDeleteConfig = func(nodeID string) {
		if v, ok := configstore.StoreMap.Load(nodeID); ok {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Stop()

			if err != nil {
				log.WithError(err).Error()
			} else {
				time.Sleep(*config.Get().ConfigDrainPeriod)
			}

			configstore.StoreMap.Delete(nodeID)
			controlplane.SnapshotCache.ClearSnapshot(nodeID)
		}
	}

	defer cms.Stop()

	serverCert, _, serverKey, _, err := certs.NewCertificate(config.AppName, certs.CertValidityMax)
	if err != nil {
		log.WithError(err).Fatal("failed to NewCertificate")
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(certs.GetLoadedRootCert())

	grpcCred := &tls.Config{
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{serverCert.Raw},
			Leaf:        serverCert,
			PrivateKey:  serverKey,
		}},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  certPool,
		RootCAs:    certPool,
	}

	ctx := context.Background()
	grpcOptions := []grpc.ServerOption{}
	grpcOptions = append(grpcOptions,
		grpc.Creds(credentials.NewTLS(grpcCred)),
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

	webServer := web.NewServer()

	go webServer.Start()
	go webServer.StartTLS()

	log.Info("grpc.address=", *config.Get().GrpcAddress)

	go startGRPC(grpcServer, lis)

	// sync manual
	go syncManual(ep, cms)

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
func syncManual(ep *endpointstore.EndpointsStore, cms *configmapsstore.ConfigMapStore) {
	for {
		time.Sleep(*config.Get().EndpointCheckPeriod)

		if err := ep.Ping(); err != nil {
			log.WithError(err).Fatal(err)
		}

		if err := cms.Ping(); err != nil {
			log.WithError(err).Fatal(err)
		}

		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.Sync()

			return true
		})
	}
}

func rotateCertificates() {
	log.Infof("rotate certificates every %s", *config.Get().SSLRotationPeriod)

	for {
		time.Sleep(*config.Get().SSLRotationPeriod)

		log.Debug("Start rotating certificates")

		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			if err := cs.LoadNewSecrets(); err != nil {
				log.WithError(err).Error("error in LoadNewSecrets")
			} else {
				cs.Push("LoadNewSecrets")
			}

			return true
		})
	}
}

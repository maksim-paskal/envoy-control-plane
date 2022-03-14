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
package controlplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"time"

	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

var SnapshotCache cache.SnapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, &Logger{})

var grpcServer *grpc.Server

func Init(ctx context.Context) {
	signal := make(chan struct{})
	cb := &callbacks{
		signal:   signal,
		fetches:  0,
		requests: 0,
	}

	createGrpcServer()

	als := &AccessLogService{}

	server := xds.NewServer(ctx, SnapshotCache, cb)

	accesslog.RegisterAccessLogServiceServer(grpcServer, als)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, server)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, server)
}

func createGrpcServer() {
	serverCert, _, serverKey, _, err := certs.NewCertificate([]string{config.AppName}, certs.CertValidityMax)
	if err != nil {
		log.WithError(err).Fatal()
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

	grpcServer = grpc.NewServer(grpcOptions...)
}

func Stop() {
	grpcServer.GracefulStop()
}

func Start() {
	log.Info("grpc.address=", *config.Get().GrpcAddress)

	lis, err := net.Listen("tcp", *config.Get().GrpcAddress)
	if err != nil {
		log.WithError(err).Fatal()
	}

	if err := grpcServer.Serve(lis); err != nil {
		log.WithError(err).Fatal()
	}
}

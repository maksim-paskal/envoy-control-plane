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
	"strings"

	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	log "github.com/sirupsen/logrus"
	k8scache "k8s.io/client-go/tools/cache"
)

var snapshotCache cache.SnapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, &Logger{})
var configStore map[string]*ConfigStore = make(map[string]*ConfigStore)

func loadConfigDirectory() {
	_, err := os.Stat(*appConfig.ConfigDirectory)

	if err != nil {
		log.Panic(err)
	}

	err = filepath.Walk(*appConfig.ConfigDirectory, func(path string, info os.FileInfo, errWalk error) error {

		if info.IsDir() {
			return nil
		}

		test := ConfigStore{}

		content, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}

		test.LoadText(path, string(content))

		configStore[test.config.Id] = &test

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

func loadConfigMaps(clientset *kubernetes.Clientset) {
	configNamespace := *appConfig.ConfigMapNamespace
	if len(configNamespace) == 0 {
		configNamespace = *appConfig.Namespace
	}
	log.Debugf("configNamespace=%s", configNamespace)

	infFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 0,
		informers.WithNamespace(configNamespace),
	)

	informer := infFactory.Core().V1().ConfigMaps().Informer()

	informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*v1.ConfigMap)

			label := strings.Split(*appConfig.ConfigMapLabels, "=")
			if cm.Labels[label[0]] != label[1] {
				return
			}
			for fileName, text := range cm.Data {
				test := ConfigStore{}
				test.LoadText(fileName, text)
				configStore[test.config.Id] = &test
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			log.Info("update")
			cm := cur.(*v1.ConfigMap)

			label := strings.Split(*appConfig.ConfigMapLabels, "=")
			if cm.Labels[label[0]] != label[1] {
				return
			}
			for fileName, text := range cm.Data {
				test := ConfigStore{}
				test.LoadText(fileName, text)
				configStore[test.config.Id] = &test
			}
		},
	})
	informer.Run(nil)
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
		loadConfigDirectory()
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", *appConfig.KubeconfigFile)
	if err != nil {
		log.Panic(err)
	}
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Panic(err)
	}
	if *appConfig.ReadConfigMap {
		go func() {
			loadConfigMaps(clientset)
		}()
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

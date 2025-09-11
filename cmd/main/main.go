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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/internal"
	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	"github.com/maksim-paskal/envoy-control-plane/pkg/web"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

var (
	version  = flag.Bool("version", false, "version")
	validate = flag.String("validate", "", "path to config file to validate")
)

const (
	defaultLeaseDuration = 15 * time.Second
	defaultRenewDeadline = 10 * time.Second
	defaultRetryPeriod   = 2 * time.Second
)

func main() {
	flag.Parse()

	if *version {
		fmt.Println(config.GetVersion()) //nolint:forbidigo
		os.Exit(0)
	}

	if *validate != "" {
		// validate config file
		if err := config.ValidateConfig(*validate); err != nil {
			log.Fatal(err)
		}

		log.Infof("%s OK", *validate)
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChanInterrupt := make(chan os.Signal, 1)
	signal.Notify(signalChanInterrupt, syscall.SIGINT, syscall.SIGTERM)

	log.RegisterExitHandler(func() {
		log.Info("Got exit signal...")
		cancel()

		time.Sleep(*config.Get().GracePeriod)
		os.Exit(1)
	})

	go func() {
		select {
		case <-signalChanInterrupt:
			log.Error("Got interruption signal...")
			cancel()
		case <-ctx.Done():
		}
		<-signalChanInterrupt
		os.Exit(1)
	}()

	// application initialization
	internal.Init(ctx)

	// initial master value
	metrics.LeaderElectionIsMaster.Set(0)

	// start metrics server
	go web.Start(ctx)

	if *config.Get().LeaderElection {
		// run leader election
		RunLeaderElection(ctx)
	} else {
		// run as a single instance
		start(ctx)
	}

	<-ctx.Done()

	log.Info("Stoped...")

	time.Sleep(*config.Get().GracePeriod)
}

func start(ctx context.Context) {
	internal.Start(ctx)

	// start controlplane
	controlplane.Init(ctx)

	go controlplane.Start(ctx)

	go web.StartTLS(ctx)
}

func RunLeaderElection(ctx context.Context) {
	if len(*config.Get().Namespace) == 0 {
		log.Fatal("-namespace is not set")
	}

	if len(*config.Get().PodName) == 0 {
		log.Fatal("-pod is not set")
	}

	lock := GetLeaseLock(*config.Get().Namespace, *config.Get().PodName)

	go leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   defaultLeaseDuration,
		RenewDeadline:   defaultRenewDeadline,
		RetryPeriod:     defaultRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				metrics.LeaderElectionIsMaster.Set(1)
				start(ctx)
			},
			OnStoppedLeading: func() {
				log.Fatal("leader election lost")
			},
		},
	})
}

func GetLeaseLock(podNamespace string, podName string) *resourcelock.LeaseLock {
	clientset := api.Client.KubeClient()

	return &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      config.AppName,
			Namespace: podNamespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: podName,
		},
	}
}

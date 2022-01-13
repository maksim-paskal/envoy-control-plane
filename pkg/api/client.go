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
package api

import (
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	k8sMetrics "k8s.io/client-go/tools/metrics"
)

const defaultResync = 0

type client struct {
	stopCh     chan struct{}
	clientset  *kubernetes.Clientset
	restconfig *rest.Config
	factory    informers.SharedInformerFactory
}

var Client *client

func Init() error {
	var err error

	Client, err = newClient()
	if err != nil {
		return errors.Wrap(err, "error creating newClient")
	}

	Client.RunAndWait()

	return nil
}

func newClient() (*client, error) {
	client := client{
		stopCh: make(chan struct{}),
	}

	var err error

	k8sMetrics.Register(k8sMetrics.RegisterOpts{
		RequestResult:  &requestResult{},
		RequestLatency: &requestLatency{},
	})

	if len(*config.Get().KubeConfigFile) > 0 {
		client.restconfig, err = clientcmd.BuildConfigFromFlags("", *config.Get().KubeConfigFile)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("No kubeconfig file use incluster")
		client.restconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	client.clientset, err = kubernetes.NewForConfig(client.restconfig)
	if err != nil {
		log.WithError(err).Fatal()
	}

	if *config.Get().WatchNamespaced {
		log.Infof("start namespaced, namespace=%s", *config.Get().Namespace)

		client.factory = informers.NewSharedInformerFactoryWithOptions(
			client.clientset,
			defaultResync,
			informers.WithNamespace(*config.Get().Namespace),
		)
	} else {
		client.factory = informers.NewSharedInformerFactoryWithOptions(
			client.clientset,
			defaultResync,
			informers.WithNamespace(*config.Get().Namespace),
		)
	}

	return &client, nil
}

func (c *client) KubeFactory() informers.SharedInformerFactory { //nolint:ireturn
	return c.factory
}

func (c *client) KubeClient() *kubernetes.Clientset {
	return c.clientset
}

func (c *client) RunAndWait() {
	c.factory.Start(c.stopCh)
	c.factory.WaitForCacheSync(c.stopCh)
}

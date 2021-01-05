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
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EndpointsStore struct {
	clientset   *kubernetes.Clientset
	informer    cache.SharedIndexInformer
	factory     informers.SharedInformerFactory
	stopCh      chan struct{}
	onNewPod    func(pod *v1.Pod)
	onDeletePod func(pod *v1.Pod)
}

func newEndpointsStore(clientset *kubernetes.Clientset) *EndpointsStore {
	es := EndpointsStore{
		clientset: clientset,
	}

	go func() {
		if *appConfig.WatchNamespaced {
			log.Debugf("start namespaced %s", *appConfig.Namespace)
			es.factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
				informers.WithNamespace(*appConfig.Namespace),
			)
		} else {
			es.factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
			)
		}

		es.informer = es.factory.Core().V1().Pods().Informer()
		es.stopCh = make(chan struct{})

		defer close(es.stopCh)

		es.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				go es.onNewPod(pod)
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				pod := newObj.(*v1.Pod)
				go es.onNewPod(pod)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				go es.onDeletePod(pod)
			},
		})

		es.factory.Start(es.stopCh)
		es.factory.WaitForCacheSync(es.stopCh)

		go es.informer.Run(es.stopCh)

		if !cache.WaitForCacheSync(es.stopCh, es.informer.HasSynced) {
			log.WithError(ErrTimeout).Fatal()

			return
		}

		<-es.stopCh
	}()

	return &es
}

func (es *EndpointsStore) Stop() {
	close(es.stopCh)
}

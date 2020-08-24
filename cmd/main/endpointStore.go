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
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EndpointsStore struct {
	clientset *kubernetes.Clientset
	informer  cache.SharedIndexInformer
	stopCh    chan struct{}
	onNewPod  func(pod *v1.Pod)
}

func newEndpointsStore(clientset *kubernetes.Clientset) *EndpointsStore {
	es := EndpointsStore{
		clientset: clientset,
	}
	go func() {
		var factory informers.SharedInformerFactory
		if *appConfig.WatchNamespaced {
			factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
				informers.WithNamespace(*appConfig.Namespace),
			)
		} else {
			factory = informers.NewSharedInformerFactoryWithOptions(
				es.clientset, 0,
			)
		}

		es.informer = factory.Core().V1().Pods().Informer()
		es.stopCh = make(chan struct{})

		es.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				es.newMessage(pod)
			},
			UpdateFunc: func(oldObj interface{}, newObj interface{}) {
				pod := newObj.(*v1.Pod)
				es.newMessage(pod)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				es.newMessage(pod)
			},
		})
		es.informer.Run(es.stopCh)
	}()
	return &es
}

func (es *EndpointsStore) Stop() {
	close(es.stopCh)
}

func (es *EndpointsStore) newMessage(pod *v1.Pod) {
	es.onNewPod(pod)
}

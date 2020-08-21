package main

import (
	log "github.com/sirupsen/logrus"
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
			if len(*appConfig.Namespace) == 0 {
				log.Panic("no namespace")
			}
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

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
package endpointstore

import (
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type EndpointsStore struct {
	informer    cache.SharedIndexInformer
	lister      listerv1.PodLister
	stopCh      chan struct{}
	OnNewPod    func(pod *v1.Pod)
	OnDeletePod func(pod *v1.Pod)
	log         *log.Entry
}

func New() *EndpointsStore {
	es := EndpointsStore{
		log: log.WithFields(log.Fields{
			"type": "EndpointsStore",
		}),
	}

	go es.init()

	return &es
}

func (es *EndpointsStore) init() {
	defer runtime.HandleCrash()

	es.informer = api.Client.KubeFactory().Core().V1().Pods().Informer()
	es.lister = api.Client.KubeFactory().Core().V1().Pods().Lister()
	es.stopCh = make(chan struct{})

	defer close(es.stopCh)

	es.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			metrics.EndpointstoreAddFunc.Inc()

			log.Debug("AddFunc")
			pod, ok := obj.(*v1.Pod)
			if !ok {
				es.log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			go es.OnNewPod(pod)
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			metrics.EndpointstoreUpdateFunc.Inc()

			log.Debug("UpdateFunc")
			pod, ok := newObj.(*v1.Pod)
			if !ok {
				es.log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			go es.OnNewPod(pod)
		},
		DeleteFunc: func(obj interface{}) {
			metrics.EndpointstoreDeleteFunc.Inc()

			log.Debug("DeleteFunc")
			pod, ok := obj.(*v1.Pod)
			if !ok {
				es.log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			go es.OnDeletePod(pod)
		},
	})

	err := es.informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		es.log.WithError(err).Fatal()
	})
	if err != nil {
		es.log.WithError(err).Fatal()
	}

	go es.informer.Run(es.stopCh)

	if !cache.WaitForCacheSync(es.stopCh, es.informer.HasSynced) {
		es.log.WithError(errTimeout).Fatal()

		return
	}

	if *config.Get().EndpointstoreWaitForPod {
		for {
			pods, err := es.lister.List(labels.Everything())
			if err != nil {
				es.log.WithError(err).Error("error listing pods")
			}

			if len(pods) > 0 {
				break
			}

			log.Info("waiting for pods...")
			time.Sleep(time.Second)
		}
	}

	<-es.stopCh
}

func (es *EndpointsStore) Ping() error {
	_, err := es.lister.List(labels.Everything())
	if err != nil {
		return errors.Wrap(err, "error listing pods")
	}

	return nil
}

func (es *EndpointsStore) Stop() {
	close(es.stopCh)
}

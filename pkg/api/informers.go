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
	"context"
	"reflect"

	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/metrics"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const unknown = "unknown"

var (
	ctx = context.Background()

	podInformer    cache.SharedIndexInformer
	podLister      listerv1.PodLister
	nodeInformer   cache.SharedIndexInformer
	nodeLister     listerv1.NodeLister
	configInformer cache.SharedIndexInformer
	configLister   listerv1.ConfigMapLister

	OnNewPod       func(pod *v1.Pod)
	OnDeletePod    func(pod *v1.Pod)
	OnNewConfig    func(*v1.ConfigMap)
	OnDeleteConfig func(*v1.ConfigMap)
)

func (c *client) RunKubeInformers() {
	defer runtime.HandleCrash()

	podInformer = Client.KubeFactory().Core().V1().Pods().Informer()
	podLister = Client.KubeFactory().Core().V1().Pods().Lister()

	nodeInformer = Client.KubeFactory().Core().V1().Nodes().Informer()
	nodeLister = Client.KubeFactory().Core().V1().Nodes().Lister()

	configInformer = Client.KubeFactory().Core().V1().ConfigMaps().Informer()
	configLister = Client.KubeFactory().Core().V1().ConfigMaps().Lister()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			metrics.EndpointstoreAddFunc.Inc()

			log.Debug("podInformer.AddFunc")
			pod, ok := obj.(*v1.Pod)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			if OnNewPod != nil {
				OnNewPod(pod)
			}
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			metrics.EndpointstoreUpdateFunc.Inc()

			log.Debug("podInformer.UpdateFunc")
			pod, ok := newObj.(*v1.Pod)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			if OnNewPod != nil {
				OnNewPod(pod)
			}
		},
		DeleteFunc: func(obj interface{}) {
			metrics.EndpointstoreDeleteFunc.Inc()

			log.Debug("podInformer.DeleteFunc")
			pod, ok := obj.(*v1.Pod)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.Pod)")
			}

			if OnDeletePod != nil {
				OnDeletePod(pod)
			}
		},
	})

	configInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			metrics.ConfigmapsstoreAddFunc.Inc()

			log.Debug("configInformer.AddFunc")
			cm, ok := obj.(*v1.ConfigMap)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.ConfigMap)")
			}

			if OnNewConfig != nil {
				OnNewConfig(cm)
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			metrics.ConfigmapsstoreUpdateFunc.Inc()

			log.Debug("configInformer.UpdateFunc")
			cm, ok := cur.(*v1.ConfigMap)
			if !ok {
				log.WithError(errAssertion).Fatal("cur.(*v1.ConfigMap)")
			}

			curConfig, ok := cur.(*v1.ConfigMap)
			if !ok {
				log.WithError(errAssertion).Fatal("cur.(*v1.ConfigMap)")
			}

			oldConfig, ok := old.(*v1.ConfigMap)
			if !ok {
				log.WithError(errAssertion).Fatal("old.(*v1.ConfigMap)")
			}

			if reflect.DeepEqual(curConfig.Data, oldConfig.Data) {
				return
			}

			if OnNewConfig != nil {
				OnNewConfig(cm)
			}
		},
		DeleteFunc: func(obj interface{}) {
			metrics.ConfigmapsstoreDeleteFunc.Inc()

			log.Debug("configInformer.DeleteFunc")
			cm, ok := obj.(*v1.ConfigMap)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.ConfigMap)")
			}

			if OnDeleteConfig != nil {
				OnDeleteConfig(cm)
			}
		},
	})

	err := podInformer.SetWatchErrorHandler(watchErrors)
	if err != nil {
		log.WithError(err).Fatal()
	}

	err = nodeInformer.SetWatchErrorHandler(watchErrors)
	if err != nil {
		log.WithError(err).Fatal()
	}

	err = configInformer.SetWatchErrorHandler(watchErrors)
	if err != nil {
		log.WithError(err).Fatal()
	}

	go podInformer.Run(c.stopCh)
	go nodeInformer.Run(c.stopCh)
	go configInformer.Run(c.stopCh)

	if !cache.WaitForCacheSync(c.stopCh, podInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	if !cache.WaitForCacheSync(c.stopCh, nodeInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	if !cache.WaitForCacheSync(c.stopCh, configInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}
}

func watchErrors(r *cache.Reflector, err error) {
	log.WithError(err).Fatal()
}

func ListPods() (ret []*v1.Pod, err error) {
	return podLister.List(labels.Everything())
}

func ListConfigMaps() (ret []*v1.ConfigMap, err error) {
	return configLister.List(labels.Everything())
}

func GetNode(name string) (*v1.Node, error) {
	return nodeLister.Get(name)
}

func GetZoneByPodName(namespace string, pod string) string {
	podInfo, err := Client.KubeClient().CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Error()

		return unknown
	}

	nodeInfo, err := Client.KubeClient().CoreV1().Nodes().Get(ctx, podInfo.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Error()

		return unknown
	}

	zone := nodeInfo.Labels[*config.Get().NodeZoneLabel]

	if len(zone) == 0 {
		return unknown
	}

	return zone
}

func (c *client) Stop() {
	close(c.stopCh)
}

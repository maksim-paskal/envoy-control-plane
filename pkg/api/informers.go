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
	"strings"
	"time"

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

const (
	unknown           = "unknown"
	informersSyncTime = 5 * time.Second
)

var (
	podInformer       cache.SharedIndexInformer
	podLister         listerv1.PodLister
	nodeInformer      cache.SharedIndexInformer
	nodeLister        listerv1.NodeLister
	configInformer    cache.SharedIndexInformer
	configLister      listerv1.ConfigMapLister
	endpointsInformer cache.SharedIndexInformer
	endpointsLister   listerv1.EndpointsLister

	OnNewPod       func(pod *v1.Pod)
	OnDeletePod    func(pod *v1.Pod)
	OnNewConfig    func(*v1.ConfigMap)
	OnDeleteConfig func(*v1.ConfigMap)
	OnNewEndpoints func(pod *v1.Endpoints)
)

func (c *client) RunKubeInformers(ctx context.Context) {
	defer runtime.HandleCrash()

	podInformer = Client.KubeFactory().Core().V1().Pods().Informer()
	podLister = Client.KubeFactory().Core().V1().Pods().Lister()

	nodeInformer = Client.KubeFactory().Core().V1().Nodes().Informer()
	nodeLister = Client.KubeFactory().Core().V1().Nodes().Lister()

	configInformer = Client.KubeFactory().Core().V1().ConfigMaps().Informer()
	configLister = Client.KubeFactory().Core().V1().ConfigMaps().Lister()

	endpointsInformer = Client.KubeFactory().Core().V1().Endpoints().Informer()
	endpointsLister = Client.KubeFactory().Core().V1().Endpoints().Lister()

	_, _ = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	_, _ = configInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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

			// do not update config if there is no change in data or annotations
			if reflect.DeepEqual(curConfig.Data, oldConfig.Data) && reflect.DeepEqual(curConfig.Annotations, oldConfig.Annotations) { //nolint:lll
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

	_, _ = endpointsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			log.Debug("endpointsInformer.AddFunc")
			endpoints, ok := obj.(*v1.Endpoints)
			if !ok {
				log.WithError(errAssertion).Fatal("obj.(*v1.Endpoints)")
			}

			if OnNewEndpoints != nil {
				OnNewEndpoints(endpoints)
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			log.Debug("endpointsInformer.UpdateFunc")
			endpoints, ok := cur.(*v1.Endpoints)
			if !ok {
				log.WithError(errAssertion).Fatal("cur.(*v1.Endpoints)")
			}

			if OnNewEndpoints != nil {
				OnNewEndpoints(endpoints)
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

	err = endpointsInformer.SetWatchErrorHandler(watchErrors)
	if err != nil {
		log.WithError(err).Fatal()
	}

	go nodeInformer.Run(c.stopCh)

	log.Infof("Waiting %s for syncing informers cache...", informersSyncTime)
	time.Sleep(informersSyncTime)

	go podInformer.Run(c.stopCh)
	go configInformer.Run(c.stopCh)
	go endpointsInformer.Run(c.stopCh)

	if !cache.WaitForCacheSync(c.stopCh, podInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	if !cache.WaitForCacheSync(c.stopCh, nodeInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	if !cache.WaitForCacheSync(c.stopCh, configInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	if !cache.WaitForCacheSync(c.stopCh, endpointsInformer.HasSynced) {
		log.WithError(errTimeout).Fatal()
	}

	go func() {
		<-ctx.Done()

		close(c.stopCh)
	}()
}

func watchErrors(_ *cache.Reflector, err error) {
	log.WithError(err).Fatal()
}

func GetPod(namespace, name string) (*v1.Pod, error) {
	return podLister.Pods(namespace).Get(name)
}

func ListPods(selectorSet map[string]string) ([]*v1.Pod, error) {
	selector := labels.Set(selectorSet).AsSelector()

	return podLister.List(selector)
}

func ListConfigMaps() ([]*v1.ConfigMap, error) {
	return configLister.List(labels.Everything())
}

func GetNode(name string) (*v1.Node, error) {
	return nodeLister.Get(name)
}

func GetEndpoint(name string) (*v1.Endpoints, error) {
	endpoints, err := endpointsLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, endpoint := range endpoints {
		// for canary services default behavior is to disable them
		// unless annotation envoy-control-plane/canary.enabled=true is set
		if strings.HasSuffix(name, config.CanarySuffix) {
			if isEnabled, ok := endpoint.Annotations[config.AnnotationCanaryEnabled]; !(ok && isEnabled == "true") {
				continue
			}
		}

		if endpoint.Name == name {
			return endpoint, nil
		}
	}

	// nothing found
	return nil, nil //nolint:nilnil
}

func GetZoneByPodName(ctx context.Context, namespace string, pod string) string {
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

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
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"
)

type ConfigMapStore struct {
	stopCh      chan struct{}
	onNewConfig func(*ConfigType)
}

func newConfigMapStore(clientset *kubernetes.Clientset) *ConfigMapStore {
	cms := ConfigMapStore{}

	go func() {
		infFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 0,
			informers.WithNamespace(*appConfig.Namespace),
		)

		informer := infFactory.Core().V1().ConfigMaps().Informer()

		informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cm := obj.(*v1.ConfigMap)

				cms.CheckData(cm)
			},
			UpdateFunc: func(old, cur interface{}) {
				curConfig := cur.(*v1.ConfigMap)
				oldConfig := old.(*v1.ConfigMap)

				if reflect.DeepEqual(curConfig.Data, oldConfig.Data) {
					return
				}

				cms.CheckData(curConfig)
			},
		})
		cms.stopCh = make(chan struct{})
		informer.Run(cms.stopCh)
	}()
	return &cms
}

func (cms *ConfigMapStore) CheckConfigMapLabels(cm *v1.ConfigMap) bool {
	label := strings.Split(*appConfig.ConfigMapLabels, "=")

	return (cm.Labels[label[0]] == label[1])
}
func (cms *ConfigMapStore) CheckData(cm *v1.ConfigMap) {
	if !cms.CheckConfigMapLabels(cm) {
		return
	}

	for fileName, text := range cm.Data {
		config := parseConfigYaml(fileName, text)

		config.configNamespace = cm.Namespace

		cms.onNewConfig(config)
	}
}

func (cms *ConfigMapStore) Stop() {
	close(cms.stopCh)
}

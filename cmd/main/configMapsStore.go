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
	"io/ioutil"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"
)

type ConfigMapStore struct {
	stopCh chan struct{}
}

func newConfigMapStore(clientset *kubernetes.Clientset) *ConfigMapStore {
	cms := ConfigMapStore{}

	configNamespace := *appConfig.ConfigMapNamespace
	if len(configNamespace) == 0 {
		configNamespace = *appConfig.Namespace
	}
	log.Debugf("configNamespace=%s", configNamespace)

	go func() {
		infFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 0,
			informers.WithNamespace(configNamespace),
		)

		informer := infFactory.Core().V1().ConfigMaps().Informer()

		informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cm := obj.(*v1.ConfigMap)

				if !cms.CheckConfigMapLabels(cm) {
					return
				}

				for fileName, text := range cm.Data {
					err := ioutil.WriteFile(filepath.Join(*appConfig.RuntimeDir, fileName), []byte(text), 0644)
					if err != nil {
						log.Error(err)
						return
					}
					loadConfigDirectory(*appConfig.RuntimeDir)
				}
			},
			UpdateFunc: func(old, cur interface{}) {
				log.Info("update")
				cm := cur.(*v1.ConfigMap)

				if !cms.CheckConfigMapLabels(cm) {
					return
				}

				for fileName, text := range cm.Data {
					err := ioutil.WriteFile(filepath.Join(*appConfig.RuntimeDir, fileName), []byte(text), 0644)
					if err != nil {
						log.Error(err)
						return
					}
					loadConfigDirectory(*appConfig.RuntimeDir)
				}
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

func (cms *ConfigMapStore) Stop() {
	close(cms.stopCh)
}

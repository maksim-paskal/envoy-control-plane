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
	"fmt"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	k8scache "k8s.io/client-go/tools/cache"
)

type ConfigMapStore struct {
	stopCh         chan struct{}
	onNewConfig    func(*ConfigType)
	onDeleteConfig func(string)
	log            *log.Entry
}

func newConfigMapStore(clientset *kubernetes.Clientset) *ConfigMapStore {
	cms := ConfigMapStore{
		log: log.WithFields(log.Fields{
			"type": "ConfigMapStore",
		}),
	}

	go func() {
		var factory informers.SharedInformerFactory
		if *appConfig.WatchNamespaced {
			cms.log.Debugf("start namespaced %s", *appConfig.Namespace)
			factory = informers.NewSharedInformerFactoryWithOptions(
				clientset, 0,
				informers.WithNamespace(*appConfig.Namespace),
			)
		} else {
			factory = informers.NewSharedInformerFactoryWithOptions(
				clientset, 0,
			)
		}

		informer := factory.Core().V1().ConfigMaps().Informer()

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
			DeleteFunc: func(obj interface{}) {
				cm := obj.(*v1.ConfigMap)

				cms.deleteUnusedConfig(cm)
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
func (cms *ConfigMapStore) deleteUnusedConfig(cm *v1.ConfigMap) {
	if !cms.CheckConfigMapLabels(cm) {
		return
	}

	for nodeId := range cm.Data {
		go cms.onDeleteConfig(nodeId)
	}

}
func (cms *ConfigMapStore) CheckData(cm *v1.ConfigMap) {
	if !cms.CheckConfigMapLabels(cm) {
		return
	}

	for nodeId, text := range cm.Data {
		log := cms.log.WithFields(log.Fields{
			"configMapName":      cm.Name,
			"configMapNamespace": cm.Namespace,
			"nodeId":             nodeId,
		})

		config, err := parseConfigYaml(nodeId, text, nil)
		if err != nil {
			log.Errorf("error parsing %s: %s", nodeId, err)
		} else {
			if len(config.Id) == 0 {
				config.Id = nodeId
			}
			config.ConfigMapName = cm.Name
			config.ConfigMapNamespace = cm.Namespace

			if config.UseVersionLabel && len(cm.Labels["version"]) > 0 {
				log.Debug("update Id, using UseVersionLabel")
				config.VersionLabel = cm.Labels["version"]
				config.Id = fmt.Sprintf("%s-%s", config.Id, config.VersionLabel)
			}

			for i := 0; i < len(config.Kubernetes); i++ {
				if len(config.Kubernetes[i].Namespace) == 0 {
					log.Debug("namespace not set - using configmap namespace")
					config.Kubernetes[i].Namespace = cm.Namespace
				}
				if config.Kubernetes[i].UseVersionLabel && len(config.VersionLabel) > 0 {
					log.Debug("add selector, using Kubernetes.UseVersionLabel")
					config.Kubernetes[i].Selector["version"] = config.VersionLabel
				}
			}
			go cms.onNewConfig(&config)
		}
	}
}

func (cms *ConfigMapStore) Stop() {
	close(cms.stopCh)
}

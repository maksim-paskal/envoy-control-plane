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
package configmapsstore

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type ConfigMapStore struct {
	stopCh         chan struct{}
	OnNewConfig    func(*config.ConfigType)
	OnDeleteConfig func(string)
	log            *log.Entry
	factory        informers.SharedInformerFactory
	informer       cache.SharedIndexInformer
}

func New() *ConfigMapStore {
	cms := ConfigMapStore{
		log: log.WithFields(log.Fields{
			"type": "ConfigMapStore",
		}),
	}

	go cms.init()

	return &cms
}

func (cms *ConfigMapStore) init() {
	defer runtime.HandleCrash()

	if *config.Get().WatchNamespaced {
		cms.log.Infof("start namespaced, namespace=%s", *config.Get().Namespace)
		cms.factory = informers.NewSharedInformerFactoryWithOptions(
			api.Clientset, 0,
			informers.WithNamespace(*config.Get().Namespace),
		)
	} else {
		cms.factory = informers.NewSharedInformerFactoryWithOptions(
			api.Clientset, 0,
		)
	}

	cms.informer = cms.factory.Core().V1().ConfigMaps().Informer()

	cms.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm, ok := obj.(*v1.ConfigMap)
			if !ok {
				cms.log.WithError(errAssertion).Fatal("obj.(*v1.ConfigMap)")
			}

			cms.CheckData(cm)
		},
		UpdateFunc: func(old, cur interface{}) {
			curConfig, ok := cur.(*v1.ConfigMap)
			if !ok {
				cms.log.WithError(errAssertion).Fatal("cur.(*v1.ConfigMap)")
			}

			oldConfig, ok := old.(*v1.ConfigMap)
			if !ok {
				cms.log.WithError(errAssertion).Fatal("old.(*v1.ConfigMap)")
			}

			if reflect.DeepEqual(curConfig.Data, oldConfig.Data) {
				return
			}

			cms.CheckData(curConfig)
		},
		DeleteFunc: func(obj interface{}) {
			cm, ok := obj.(*v1.ConfigMap)
			if !ok {
				cms.log.WithError(errAssertion).Fatal("obj.(*v1.ConfigMap)")
			}

			cms.deleteUnusedConfig(cm)
		},
	})

	err := cms.informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		cms.log.WithError(err).Fatal()
	})
	if err != nil {
		cms.log.WithError(err).Fatal()
	}

	cms.stopCh = make(chan struct{})

	defer close(cms.stopCh)

	cms.factory.Start(cms.stopCh)
	cms.factory.WaitForCacheSync(cms.stopCh)

	go cms.informer.Run(cms.stopCh)

	if !cache.WaitForCacheSync(cms.stopCh, cms.informer.HasSynced) {
		cms.log.WithError(errTimeout).Fatal()

		return
	}

	<-cms.stopCh
}

func (cms *ConfigMapStore) CheckConfigMapLabels(cm *v1.ConfigMap) bool {
	label := strings.Split(*config.Get().ConfigMapLabels, "=")

	return (cm.Labels[label[0]] == label[1])
}

func (cms *ConfigMapStore) deleteUnusedConfig(cm *v1.ConfigMap) {
	if !cms.CheckConfigMapLabels(cm) {
		return
	}

	for nodeID := range cm.Data {
		go cms.OnDeleteConfig(nodeID)
	}
}

func (cms *ConfigMapStore) CheckData(cm *v1.ConfigMap) {
	if !cms.CheckConfigMapLabels(cm) {
		return
	}

	for nodeID, text := range cm.Data {
		log := cms.log.WithFields(log.Fields{
			"configMapName":      cm.Name,
			"configMapNamespace": cm.Namespace,
			"nodeID":             nodeID,
		})

		config, err := config.ParseConfigYaml(nodeID, text, nil)
		if err != nil {
			cms.log.WithError(err).Errorf("error parsing %s: %s", nodeID, text)
		} else {
			if len(config.ID) == 0 {
				config.ID = nodeID
			}
			if len(config.Name) == 0 {
				config.Name = config.ID
			}
			config.ConfigMapName = cm.Name
			config.ConfigMapNamespace = cm.Namespace

			if config.UseVersionLabel && len(cm.Labels["version"]) > 0 {
				log.Debug("update Id, using UseVersionLabel")
				config.VersionLabel = cm.Labels["version"]
				config.ID = fmt.Sprintf("%s-%s", config.ID, config.VersionLabel)
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
			go cms.OnNewConfig(&config)
		}
	}
}

func (cms *ConfigMapStore) Stop() {
	close(cms.stopCh)
}

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
	"strings"
	"sync"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

var mutex sync.Mutex

func checkConfigMapLabels(cm *v1.ConfigMap) bool {
	label := strings.Split(*config.Get().ConfigMapLabels, "=")

	return (cm.Labels[label[0]] == label[1])
}

func NewConfigMap(cm *v1.ConfigMap) error {
	if !checkConfigMapLabels(cm) {
		return nil
	}

	mutex.Lock()
	defer mutex.Unlock()

	for nodeID, text := range cm.Data {
		config, err := config.ParseConfigYaml(nodeID, text, nil)
		if err != nil {
			return err
		}

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

		if v, ok := configstore.StoreMap.Load(config.ID); ok {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				return errAssertion
			}

			cs.Stop()
		}

		log.Infof("Create configStore %s", config.ID)

		newConfigStore, err := configstore.New(&config)
		if err != nil {
			return err
		}

		configstore.StoreMap.Store(config.ID, newConfigStore)
	}

	return nil
}

func DeleteConfigMap(cm *v1.ConfigMap) {
	configstore.StoreMap.Range(func(key interface{}, value interface{}) bool {
		cs, ok := value.(*configstore.ConfigStore)

		if !ok {
			log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
		}

		if cs.Config.ConfigMapName == cm.Name && cs.Config.ConfigMapNamespace == cm.Namespace {
			cs.Stop()

			time.Sleep(*config.Get().ConfigDrainPeriod)

			controlplane.SnapshotCache.ClearSnapshot(cs.Config.ID)
			configstore.StoreMap.Delete(key)
		}

		return true
	})
}

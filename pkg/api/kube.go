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

	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const unknown = "unknown"

var ctx = context.Background()

func GetZone(namespace string, pod string) string {
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

func GetNode(nodeName string) (*v1.Node, error) {
	nodeInfo, err := Client.KubeClient().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "cs.ep.clientset.CoreV1().Nodes().Get")
	}

	return nodeInfo, nil
}

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
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getKubernetesClient() (*kubernetes.Clientset, error) {
	var (
		kubeconfig *rest.Config
		err        error
	)

	if len(*appConfig.KubeConfigFile) > 0 {
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", *appConfig.KubeConfigFile)
		if err != nil {
			return nil, errors.Wrap(err, "error in BuildConfigFromFlags="+*appConfig.KubeConfigFile)
		}
	} else {
		kubeconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error in InClusterConfig")
		}
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, errors.Wrap(err, "error in NewForConfig")
	}

	return clientset, nil
}

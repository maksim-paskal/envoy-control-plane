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
package main_test

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/internal"
	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ctx                       = context.Background()
	namespace                 = ""
	maxTryCount               = 0
	initialPodCount           = flag.Int("initialPodCount", 10, "")
	errNotFound               = errors.New("configname not found")
	errTypeError              = errors.New("can not convert types")
	errIPNotFound             = errors.New("IP not found")
	errPodsNotFound           = errors.New("PODS not found")
	errConfigMapMustBeDeleted = errors.New("configmap must be deleted")
	errLocalIPNotFound        = errors.New("local IPs not found")
)

const (
	configMapName          = "test1-id"
	localEndpointsInConfig = 2
	initialWait            = 5 * time.Second
	maxTestTryCount        = 3
)

func TestConfigMapsStore(t *testing.T) {
	t.Parallel()

	flag.Parse()

	internal.Init(t.Context())

	internal.Start(ctx)

	// initial wait
	time.Sleep(initialWait)

	namespace = *config.Get().Namespace

	if err := testConfigmapsstore(); err != nil {
		t.Fatal(err)
	}

	if maxTryCount > maxTestTryCount {
		t.Fatalf("endpoint wait time %d bigger than %d", maxTryCount, maxTestTryCount)
	} else {
		log.Infof("endpoint wait time %d", maxTryCount)
	}

	log.Info("Test finished")
}

func testConfigmapsstore() error {
	v, ok := configstore.StoreMap.Load(configMapName)
	if !ok {
		return errors.Wrap(errNotFound, configMapName)
	}

	cs, ok := v.(*configstore.ConfigStore)

	if !ok {
		return errTypeError
	}

	for podsCount := *initialPodCount; podsCount >= 0; podsCount-- {
		if err := checkPods(cs, podsCount); err != nil {
			return err
		}
	}

	return checkConfigDeletion()
}

func scaleAll(count int) error {
	if err := scaleDeploy("test-001", count); err != nil {
		return err
	}

	return scaleDeploy("test-002", count)
}

func scaleDeploy(name string, count int) error {
	deploy, err := api.Client.KubeClient().AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	replicas := int32(count) //nolint: gosec

	deploy.Spec.Replicas = &replicas

	_, err = api.Client.KubeClient().AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func checkPods(cs *configstore.ConfigStore, podCount int) error {
	log.Infof("podCount=%d", podCount)

	if err := scaleAll(podCount); err != nil {
		return err
	}

	var getLastEndpoints []string

	tryCount := 0

	// wait for pods will scale down
	for {
		time.Sleep(time.Second)

		getLastEndpoints = cs.GetLastEndpoints()

		// total pods in namespace = len(podCount) x 2
		// and 2 x local endpoints in configs
		if len(getLastEndpoints) == podCount*2+localEndpointsInConfig {
			break
		}

		log.Warnf("wait for endpoints in store len=%d,podCount=%d,tryCount=%d",
			len(getLastEndpoints),
			podCount,
			tryCount,
		)

		tryCount++
	}

	if tryCount > maxTryCount {
		maxTryCount = tryCount
	}

	ipLocalService1 := make([]string, 0)
	ipLocalService1Local := make([]string, 0)
	ipTestEnvoyService := make([]string, 0)

	for _, endpoint := range getLastEndpoints {
		data := strings.Split(endpoint, "|")

		switch data[0] {
		case "local_service1":
			if strings.HasPrefix(data[3], "127.0.0") {
				ipLocalService1Local = append(ipLocalService1Local, data[3])
			} else {
				ipLocalService1 = append(ipLocalService1, data[3])
			}
		case "test-envoy-service":
			ipTestEnvoyService = append(ipTestEnvoyService, data[3])
		}
	}

	if l := len(ipLocalService1); l != podCount {
		return errors.Wrapf(errPodsNotFound, "ipLocalService1, len=%d, podCount=%d", l, podCount)
	}

	if l := len(ipTestEnvoyService); l != podCount {
		return errors.Wrapf(errPodsNotFound, "ipTestEnvoyService, len=%d, podCount=%d", l, podCount)
	}

	if !searchPodInArray("app=test-001", ipLocalService1) {
		return errors.Wrapf(errIPNotFound, "ipLocalService1, podCount=%d", podCount)
	}

	if !searchPodInArray("app=test-002", ipTestEnvoyService) {
		return errors.Wrapf(errIPNotFound, "ipTestEnvoyService, podCount=%d", podCount)
	}

	if len(ipLocalService1Local) != localEndpointsInConfig {
		return errors.Wrapf(errLocalIPNotFound, "cluster must have 2 local IP, podCount=%d", podCount)
	}

	return nil
}

func checkConfigDeletion() error {
	err := api.Client.KubeClient().CoreV1().ConfigMaps(namespace).Delete(ctx, configMapName, metav1.DeleteOptions{}) //nolint:lll
	if err != nil {
		return err
	}

	// wait for drain
	time.Sleep(*config.Get().ConfigDrainPeriod)
	time.Sleep(time.Second)

	_, ok := configstore.StoreMap.Load(configMapName)
	if ok {
		return errConfigMapMustBeDeleted
	}

	return nil
}

func searchPodInArray(labelSelector string, ipArray []string) bool {
	ctx := context.Background()

	list, err := api.Client.KubeClient().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		log.Error(err)

		return false
	}

	found := 0

	for _, pod := range list.Items {
		for _, ip := range ipArray {
			if pod.Status.PodIP == ip {
				found++
			}
		}
	}

	return found == len(ipArray)
}

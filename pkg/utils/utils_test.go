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
package utils_test

import (
	"testing"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/google/uuid"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/utils"
)

func TestGetConfigSnapshot(t *testing.T) {
	t.Parallel()

	c := config.ConfigType{}
	r := []types.Resource{}
	s := []tls.Secret{}

	e := endpoint.ClusterLoadAssignment{
		ClusterName: "clusterName",
	}

	r = append(r, &e)

	version := uuid.New().String()

	snapshot, err := utils.GetConfigSnapshot(version, &c, r, s)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(snapshot)

	if snapshot.Resources[0].Version != version {
		t.Fatal("not correct version")
	}
}

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
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/maksim-paskal/envoy-control-plane/internal"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/controlplane"
	"github.com/maksim-paskal/envoy-control-plane/pkg/web"
)

var version = flag.Bool("version", false, "version")

func main() {
	ctx := context.Background()

	flag.Parse()

	if *version {
		fmt.Println(config.GetVersion()) //nolint:forbidigo
		os.Exit(0)
	}

	// application initialization
	internal.Init()
	defer internal.Stop()

	// start controlplane
	controlplane.Init(ctx)
	defer controlplane.Stop()

	go controlplane.Start()

	// create web server
	go web.Start()
	go web.StartTLS()

	<-ctx.Done()
}

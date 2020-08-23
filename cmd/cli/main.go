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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"
)

var (
	action    = flag.String("action", "/api/zone", "api action")
	namespace = flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "pod namespace")
	pod       = flag.String("pod", os.Getenv("HOSTNAME"), "pod name")
	server    = flag.String("server", "envoy-control-plane", "controlplane host")
	port      = flag.Int("port", 18081, "controlplane port")
	wait      = flag.Bool("wait", true, "wait controlplane")
)

func waitForAPI() {
	for {
		resp, _ := http.Get(fmt.Sprintf("http://%s:%d/api/ready", *server, *port))

		if resp != nil && resp.StatusCode == 200 {
			return
		}
		fmt.Println("Wait for api ready...")
		time.Sleep(1 * time.Second)
	}
}
func main() {
	flag.Parse()
	if len(*namespace) == 0 {
		panic("no namespace")
	}
	if len(*pod) == 0 {
		panic("no pod")
	}

	if *wait {
		waitForAPI()
	}
	formData := url.Values{
		"namespace": {*namespace},
		"pod":       {*pod},
	}
	resp, err := http.PostForm(fmt.Sprintf("http://%s:%d%s", *server, *port, *action), formData)
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(body))

	if resp.StatusCode != 200 {
		panic("result not 200")
	}
}

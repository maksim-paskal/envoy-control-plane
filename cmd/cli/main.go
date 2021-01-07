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
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	action    = flag.String("action", "/api/zone", "api action")
	namespace = flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "pod namespace")
	pod       = flag.String("pod", os.Getenv("HOSTNAME"), "pod name")
	server    = flag.String("server", "envoy-control-plane", "controlplane host")
	port      = flag.Int("port", 18081, "controlplane port")
	wait      = flag.Bool("wait", true, "wait controlplane")
	debug     = flag.Bool("debug", false, "debug mode")
)

func waitForAPI() {
	cli := &http.Client{}
	ctx := context.Background()
	url := fmt.Sprintf("http://%s:%d/api/ready", *server, *port)

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil && *debug {
			os.Stderr.WriteString(err.Error())
		}

		resp, err := cli.Do(req)

		if err != nil && *debug {
			os.Stderr.WriteString(err.Error())
		}

		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		if resp != nil && resp.StatusCode == 200 {
			return
		}

		if *debug {
			os.Stdout.WriteString("Wait for api ready...")
		}

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

	cli := &http.Client{}
	ctx := context.Background()
	requestURL := fmt.Sprintf("http://%s:%d%s", *server, *port, *action)

	data := url.Values{
		"namespace": {*namespace},
		"pod":       {*pod},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cli.Do(req)
	if err != nil {
		panic(err)
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	os.Stdout.WriteString(string(body))

	if resp.StatusCode != http.StatusOK {
		panic("result not ok")
	}
}

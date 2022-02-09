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
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	serverPort      = 18081
	serverAdminPort = 18000
	defaultTimeout  = 10 * time.Second
)

var (
	cli            = &http.Client{}
	ctx            = context.Background()
	action         = flag.String("action", "/api/zone", "api action")
	namespace      = flag.String("namespace", os.Getenv("MY_POD_NAMESPACE"), "pod namespace")
	pod            = flag.String("pod", os.Getenv("HOSTNAME"), "pod name")
	server         = flag.String("server", "envoy-control-plane", "controlplane host")
	port           = flag.Int("port", serverPort, "controlplane port")
	wait           = flag.Bool("wait", true, "wait controlplane")
	debug          = flag.Bool("debug", false, "debug mode")
	envoyLogLevel  = flag.String("envoyLogLevel", "", "set envoy log level")
	drainEnvoy     = flag.Bool("drainEnvoy", false, "drain envoy")
	timeout        = flag.Duration("timeout", defaultTimeout, "timeout to shutdown envoy")
	envoyAdminPort = flag.Int("envoyAdminPort", serverAdminPort, "envoy admin port")
	logFlags       = flag.Int("logFlags", 0, "log flags")
	tlsInsecure    = flag.Bool("tls.insecure", false, "use insecure TLS")
	tlsCA          = flag.String("tls.CA", "/certs/CA.crt", "CA certificate")
	tlsClientCrt   = flag.String("tls.Crt", "/certs/envoy.crt", "tls client certificate")
	tlsClientKey   = flag.String("tls.Key", "/certs/envoy.key", "tls client certificate key")
)

func waitForAPI() {
	url := fmt.Sprintf("https://%s:%d/api/ready", *server, *port)

	for {
		if *debug {
			log.Println("Connecting to", url)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil && *debug {
			log.Println(err.Error())
		}

		resp, err := cli.Do(req)

		if err != nil && *debug {
			log.Println(err.Error())
		}

		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		if resp != nil && resp.StatusCode == 200 {
			return
		}

		if *debug {
			log.Println("Wait for api ready...")
		}

		time.Sleep(1 * time.Second)
	}
}

func requestEnvoyAdmin(path string) {
	method := http.MethodPost
	url := fmt.Sprintf("http://127.0.0.1:%d%s", *envoyAdminPort, path)

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := cli.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	resp.Body.Close()

	log.Printf("[%s] %s", path, string(body))
}

func main() {
	flag.Parse()

	log.SetFlags(*logFlags)

	caCertPool := x509.NewCertPool()
	cliCerts := []tls.Certificate{}

	if !*tlsInsecure && len(*tlsClientCrt) > 0 && len(*tlsCA) > 0 {
		cert, err := tls.LoadX509KeyPair(*tlsClientCrt, *tlsClientKey)
		if err != nil {
			log.Fatal(err)
		}

		cliCerts = []tls.Certificate{cert}
	}

	if !*tlsInsecure && len(*tlsCA) > 0 {
		caCert, err := ioutil.ReadFile(*tlsCA)
		if err != nil {
			log.Fatal(err)
		}

		caCertPool.AppendCertsFromPEM(caCert)
	}

	cli = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				Certificates:       cliCerts,
				RootCAs:            caCertPool,
				InsecureSkipVerify: *tlsInsecure, //nolint:gosec
			},
		},
	}

	if len(*envoyLogLevel) > 0 {
		requestEnvoyAdmin(fmt.Sprintf("/logging?level=%s", *envoyLogLevel))

		return
	}

	if *drainEnvoy {
		// draining connections
		requestEnvoyAdmin("/drain_listeners?graceful")
		requestEnvoyAdmin("/healthcheck/fail")

		// wait some time
		log.Printf("Waiting %s to Envoy quit", *timeout)
		time.Sleep(*timeout)

		// shutdown envoy
		requestEnvoyAdmin("/quitquitquit")

		return
	}

	if len(*namespace) == 0 {
		log.Fatal("no namespace")
	}

	if len(*pod) == 0 {
		log.Fatal("no pod")
	}

	if *wait {
		waitForAPI()
	}

	requestURL := fmt.Sprintf("https://%s:%d%s", *server, *port, *action)

	data := url.Values{
		"namespace": {*namespace},
		"pod":       {*pod},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cli.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if resp.Body != nil {
		resp.Body.Close()
	}

	os.Stdout.WriteString(string(body))

	if resp.StatusCode != http.StatusOK {
		log.Fatal("result not ok")
	}
}

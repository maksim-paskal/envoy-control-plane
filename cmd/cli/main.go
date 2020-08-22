package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

var (
	action    = flag.String("action", "/api/zone", "api action")
	namespace = flag.String("namespace", "", "pod namespace")
	pod       = flag.String("pod", "", "pod name")
	server    = flag.String("server", "localhost", "controlplane host")
	port      = flag.Int("port", 18081, "controlplane port")
)

func main() {
	flag.Parse()
	if len(*namespace) == 0 {
		panic("no namespace")
	}
	if len(*pod) == 0 {
		panic("no pod")
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

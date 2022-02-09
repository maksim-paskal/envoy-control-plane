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
	"io/fs"
	"io/ioutil"
	"path"
	"strings"

	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	certPath := flag.String("cert.path", "certs", "path to generate certificates")
	dnsNames := flag.String("dns.names", "test", "dns names for server certificate")

	flag.Parse()

	files := make(map[string][]byte)

	if err := certs.Init(); err != nil {
		log.WithError(err).Fatal()
	}

	rootCrt := certs.GetLoadedRootCert()
	rootCrtBytes := certs.GetLoadedRootCertBytes()

	rootKey := certs.GetLoadedRootKey()

	rootKeyBytes, err := certs.GetLoadedRootKeyBytes()
	if err != nil {
		log.WithError(err).Fatal()
	}

	if len(*config.Get().SSLCrt) == 0 && len(*config.Get().SSLKey) == 0 {
		files["CA.crt"] = rootCrtBytes
		files["CA.key"] = rootKeyBytes
	}

	_, serverCrtBytes, _, serverKeyBytes, err := certs.GenServerCert(strings.Split(*dnsNames, ","), rootCrt, rootKey, certs.CertValidityMax) //nolint:lll
	if err != nil {
		log.Fatal(err)
	}

	files["server.crt"] = serverCrtBytes
	files["server.key"] = serverKeyBytes

	_, envoyCrtBytes, _, envoyKeyBytes, err := certs.GenServerCert([]string{"envoy"}, rootCrt, rootKey, certs.CertValidityMax) //nolint:lll
	if err != nil {
		log.Fatal(err)
	}

	files["envoy.crt"] = envoyCrtBytes
	files["envoy.key"] = envoyKeyBytes

	const fileMode = fs.FileMode(0o644)

	for fileName, fileContent := range files {
		filePath := path.Join(*certPath, fileName)

		log.Infof("saving file %s", filePath)

		if err = ioutil.WriteFile(filePath, fileContent, fileMode); err != nil {
			log.Fatal(err)
		}
	}

	log.Info("certificates generated")
}

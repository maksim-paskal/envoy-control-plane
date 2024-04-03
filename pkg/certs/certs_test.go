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
package certs_test

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/pkg/errors"
)

func TestCertCA(t *testing.T) {
	t.Parallel()

	_, _, _, _, err := certs.GenCARoot() //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}
}

func TestCert(t *testing.T) {
	t.Parallel()

	rootCert, _, rootKey, _, err := certs.GenCARoot()
	if err != nil {
		t.Fatal(err)
	}

	serverCert, _, _, _, err := certs.GenServerCert([]string{"test"}, rootCert, rootKey, time.Minute) //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}

	err = verifyLow(rootCert, serverCert)
	if err != nil {
		t.Fatal(err)
	}
}

func verifyLow(root, child *x509.Certificate) error {
	roots := x509.NewCertPool()
	inter := x509.NewCertPool()

	roots.AddCert(root)

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
	}

	if _, err := child.Verify(opts); err != nil {
		return errors.Wrap(err, "failed to verify certificate")
	}

	return nil
}

func TestLoadCert(t *testing.T) {
	t.Parallel()

	// generate new cert
	if err := certs.Init(); err != nil {
		t.Fatal(err)
	}

	if err := config.Load(); err != nil {
		t.Fatal(err)
	}

	// load from files
	if err := certs.Init(); err != nil {
		t.Fatal(err)
	}

	serverCert, _, _, _, err := certs.NewCertificate([]string{"test"}, time.Minute) //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}

	err = verifyLow(certs.GetLoadedRootCert(), serverCert)
	if err != nil {
		t.Fatal(err)
	}
}

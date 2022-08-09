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
package certs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	keyBits          = 2048
	CertValidity     = 7 * 24 * time.Hour
	CertValidityYear = 365 * 24 * time.Hour
	CertValidityMax  = 3000 * 24 * time.Hour
	sslMaxPathLen    = 2
)

var (
	caCert      *x509.Certificate
	caCertBytes []byte
	caKey       *rsa.PrivateKey
)

func genCert(template, parent *x509.Certificate, publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) (*x509.Certificate, []byte, error) { //nolint:lll
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, privateKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to create certificate")
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to parse certificate")
	}

	b := pem.Block{Type: "CERTIFICATE", Bytes: certBytes}
	certPEM := pem.EncodeToMemory(&b)

	return cert, certPEM, nil
}

func Init() error {
	var err error

	if len(*config.Get().SSLCrt) > 0 && len(*config.Get().SSLKey) > 0 {
		log.Infof("loading cerificate from files %s,%s", *config.Get().SSLCrt, *config.Get().SSLKey)

		caCert, caCertBytes, caKey, err = loadCAFromFiles()
	} else {
		log.Info("generate new certificate")

		caCert, caCertBytes, caKey, _, err = GenCARoot()
	}

	if err != nil {
		return err
	}

	log.Debugf("root CA\n%s", string(GetLoadedRootCertBytes()))

	return nil
}

func GetLoadedRootCert() *x509.Certificate {
	return caCert
}

func GetLoadedRootCertBytes() []byte {
	return caCertBytes
}

func GetLoadedRootKey() *rsa.PrivateKey {
	return caKey
}

func GetLoadedRootKeyBytes() ([]byte, error) {
	return exportPrivateKey(caKey)
}

func NewCertificate(dnsNames []string, certDuration time.Duration) (*x509.Certificate, []byte, *rsa.PrivateKey, []byte, error) { //nolint:lll
	return GenServerCert(dnsNames, caCert, caKey, certDuration)
}

func loadCAFromFiles() (*x509.Certificate, []byte, *rsa.PrivateKey, error) {
	certBytes, err := os.ReadFile(*config.Get().SSLCrt)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "can not load certicate")
	}

	certBlock, _ := pem.Decode(certBytes)

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "can not parse certicate")
	}

	keyBytes, err := os.ReadFile(*config.Get().SSLKey)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "can not load key")
	}

	keyBlock, _ := pem.Decode(keyBytes)

	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "can not parse key")
	}

	privateKey, ok := key.(*rsa.PrivateKey)

	if !ok {
		return nil, nil, nil, errors.New("assertion error")
	}

	return cert, certBytes, privateKey, nil
}

func GenCARoot() (*x509.Certificate, []byte, *rsa.PrivateKey, []byte, error) {
	rootTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:            []string{"US"},
			Organization:       []string{config.AppName},
			OrganizationalUnit: []string{"CA"},
			CommonName:         config.AppName,
		},
		NotBefore:             time.Now().Add(-10 * time.Second),
		NotAfter:              time.Now().Add(CertValidityMax),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            sslMaxPathLen,
	}

	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate key")
	}

	rootCert, rootCertBytes, err := genCert(&rootTemplate, &rootTemplate, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate cert")
	}

	priBytes, err := exportPrivateKey(priv)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate private key")
	}

	return rootCert, rootCertBytes, priv, priBytes, nil
}

func GenServerCert(dnsNames []string, rootCert *x509.Certificate, rootKey *rsa.PrivateKey, certDuration time.Duration) (*x509.Certificate, []byte, *rsa.PrivateKey, []byte, error) { //nolint: lll
	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate key")
	}

	serverTemplate := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(time.Now().Unix()),
		Subject: pkix.Name{
			Country:            []string{"US"},
			Organization:       []string{config.AppName},
			OrganizationalUnit: []string{"CLIENT"},
			CommonName:         dnsNames[0],
		},
		DNSNames:       dnsNames,
		NotBefore:      time.Now().Add(-10 * time.Second),
		NotAfter:       time.Now().Add(certDuration),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:           false,
		MaxPathLenZero: true,
	}

	serverCert, serverCertBytes, err := genCert(&serverTemplate, rootCert, &priv.PublicKey, rootKey)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate cert")
	}

	priBytes, err := exportPrivateKey(priv)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "Failed to generate private key")
	}

	return serverCert, serverCertBytes, priv, priBytes, nil
}

func exportPrivateKey(privkey *rsa.PrivateKey) ([]byte, error) {
	privkeyBytes, err := x509.MarshalPKCS8PrivateKey(privkey)
	if err != nil {
		return nil, err
	}

	privkeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privkeyBytes,
		},
	)

	return privkeyPem, nil
}

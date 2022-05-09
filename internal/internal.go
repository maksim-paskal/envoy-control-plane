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
package internal

import (
	"time"

	"github.com/maksim-paskal/envoy-control-plane/pkg/api"
	"github.com/maksim-paskal/envoy-control-plane/pkg/certs"
	"github.com/maksim-paskal/envoy-control-plane/pkg/config"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configmapsstore"
	"github.com/maksim-paskal/envoy-control-plane/pkg/configstore"
	logrushooksentry "github.com/maksim-paskal/logrus-hook-sentry"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

var hook *logrushooksentry.Hook

func Init() {
	var err error
	if err = config.Load(); err != nil {
		log.Fatal(err)
	}

	logLevel, err := log.ParseLevel(*config.Get().LogLevel)
	if err != nil {
		log.WithError(err).Fatal()
	}

	log.SetLevel(logLevel)

	log.Debugf("Using config:\n%s", config.String())

	if *config.Get().LogPretty {
		log.SetFormatter(&log.TextFormatter{})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if logLevel == log.DebugLevel || *config.Get().LogReportCaller {
		log.SetReportCaller(true)
	}

	if err = config.CheckConfig(); err != nil {
		log.Fatal(err)
	}

	hook, err = logrushooksentry.NewHook(logrushooksentry.Options{
		SentryDSN: *config.Get().SentryDSN,
		Release:   config.GetVersion(),
	})
	if err != nil {
		log.WithError(err).Error()
	}

	log.AddHook(hook)

	log.Infof("Starting %s...", config.GetVersion())

	if err = api.Init(); err != nil {
		log.WithError(err).Fatal()
	}

	if err = certs.Init(); err != nil {
		log.WithError(err).Fatal()
	}

	api.OnNewPod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.NewPod(pod)

			return true
		})
	}

	api.OnDeletePod = func(pod *v1.Pod) {
		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")
			}

			cs.DeletePod(pod)

			return true
		})
	}

	api.OnNewConfig = func(cm *v1.ConfigMap) {
		if err := configmapsstore.NewConfigMap(cm); err != nil {
			log.WithError(err).Error()
		}
	}

	api.OnDeleteConfig = func(cm *v1.ConfigMap) {
		configmapsstore.DeleteConfigMap(cm)
	}

	api.Client.RunKubeInformers()

	// shedule all jobs
	schedule()
}

func Stop() {
	api.Client.Stop()
	hook.Stop()
}

func schedule() {
	// rotate certificates
	go rotateCertificates()

	// sync all endpoints
	go syncAll()
}

// sync all endpoints in configs with endpointstore.
func syncAll() {
	log.Infof("syncAll every %s", *config.Get().EndpointCheckPeriod)

	for {
		time.Sleep(*config.Get().EndpointCheckPeriod)

		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")

				return true
			}

			cs.Sync()

			return true
		})
	}
}

func rotateCertificates() {
	log.Infof("syncAll every %s", *config.Get().SSLRotationPeriod)

	for {
		time.Sleep(*config.Get().SSLRotationPeriod)

		configstore.StoreMap.Range(func(k, v interface{}) bool {
			cs, ok := v.(*configstore.ConfigStore)

			if !ok {
				log.WithError(errAssertion).Fatal("v.(*ConfigStore)")

				return true
			}

			if err := cs.LoadNewSecrets(); err != nil {
				log.WithError(err).Error("error in LoadNewSecrets")

				return true
			}

			cs.Push("LoadNewSecrets")

			return true
		})
	}
}

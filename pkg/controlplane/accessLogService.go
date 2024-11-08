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
package controlplane

import (
	"time"

	alf "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accessloggrpc "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// AccessLogService buffers access logs from the remote Envoy nodes.
type AccessLogService struct{}

// StreamAccessLogs implements the access log service.
func (svc *AccessLogService) StreamAccessLogs(stream accessloggrpc.AccessLogService_StreamAccessLogsServer) error {
	var logName string

	for {
		msg, err := stream.Recv()
		if err != nil {
			return errors.Wrap(err, "error in stream.Recv()")
		}

		if msg.GetIdentifier() != nil {
			logName = msg.GetIdentifier().GetLogName()
		}

		switch entries := msg.GetLogEntries().(type) {
		case *accessloggrpc.StreamAccessLogsMessage_HttpLogs:
			for _, entry := range entries.HttpLogs.GetLogEntry() {
				if entry != nil {
					common := entry.GetCommonProperties()
					req := entry.GetRequest()
					resp := entry.GetResponse()

					if common == nil {
						common = &alf.AccessLogCommon{}
					}

					if req == nil {
						req = &alf.HTTPRequestProperties{}
					}

					if resp == nil {
						resp = &alf.HTTPResponseProperties{}
					}

					log.Infof("[%s%s] %s %s %s %d %s %s",
						logName, time.Now().Format(time.RFC3339), req.GetAuthority(), req.GetPath(), req.GetScheme(),
						resp.GetResponseCode().GetValue(), req.GetRequestId(), common.GetUpstreamCluster())
				}
			}
		case *accessloggrpc.StreamAccessLogsMessage_TcpLogs:
			for _, entry := range entries.TcpLogs.GetLogEntry() {
				if entry != nil {
					common := entry.GetCommonProperties()
					if common == nil {
						common = &alf.AccessLogCommon{}
					}

					log.Infof("[%s%s] tcp %s %s",
						logName, time.Now().Format(time.RFC3339), common.GetUpstreamLocalAddress(), common.GetUpstreamCluster())
				}
			}
		}
	}
}

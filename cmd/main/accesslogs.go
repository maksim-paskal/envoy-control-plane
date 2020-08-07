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
	"fmt"
	"time"

	alf "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v2"
	accessloggrpc "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
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
			return nil
		}
		if msg.Identifier != nil {
			logName = msg.Identifier.LogName
		}
		switch entries := msg.LogEntries.(type) {
		case *accessloggrpc.StreamAccessLogsMessage_HttpLogs:
			for _, entry := range entries.HttpLogs.LogEntry {
				if entry != nil {
					common := entry.CommonProperties
					req := entry.Request
					resp := entry.Response
					if common == nil {
						common = &alf.AccessLogCommon{}
					}
					if req == nil {
						req = &alf.HTTPRequestProperties{}
					}
					if resp == nil {
						resp = &alf.HTTPResponseProperties{}
					}
					log.Infof(fmt.Sprintf("[%s%s] %s %s %s %d %s %s",
						logName, time.Now().Format(time.RFC3339), req.Authority, req.Path, req.Scheme,
						resp.ResponseCode.GetValue(), req.RequestId, common.UpstreamCluster))
				}
			}
		case *accessloggrpc.StreamAccessLogsMessage_TcpLogs:
			for _, entry := range entries.TcpLogs.LogEntry {
				if entry != nil {
					common := entry.CommonProperties
					if common == nil {
						common = &alf.AccessLogCommon{}
					}
					log.Infof(fmt.Sprintf("[%s%s] tcp %s %s",
						logName, time.Now().Format(time.RFC3339), common.UpstreamLocalAddress, common.UpstreamCluster))
				}
			}
		}
	}
}

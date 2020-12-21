
FROM envoyproxy/envoy:v1.15.2

ADD --chown=envoy:envoy https://github.com/maksim-paskal/go-template/releases/download/v0.0.8/go-template-linux-amd64 /usr/local/bin/go-template
ADD --chown=envoy:envoy https://github.com/maksim-paskal/jaeger-client-cpp/releases/download/v0.5.0/libjaegertracing_plugin.so /usr/local/lib/libjaegertracing_plugin.so
ADD --chown=envoy:envoy https://github.com/maksim-paskal/envoy-control-plane/releases/download/cli/cli-linux /usr/local/bin/cli

COPY ./entrypoint.sh /entrypoint.sh
COPY ./envoy.defaults /envoy.defaults/

RUN touch /tmp/checksum \
&& echo "473ce4db569c2adfb6fc52a35582310c66164a241b7d7c5cc514d334757b682a /usr/local/lib/libjaegertracing_plugin.so" >> /tmp/checksum \
&& echo "20e7dd0c1c32f06ceaab47f1c561557384bc5015ff1ad7bfad6ca6f10733b34d /usr/local/bin/go-template" >> /tmp/checksum \
&& echo "38c7da3317c05df8a4d9c17a9a517ff9363f8de5b5ad13b4685ce5e8c3d32525 /usr/local/bin/cli" >> /tmp/checksum \
&& cat /tmp/checksum \
&& sha256sum -c /tmp/checksum \
&& rm /tmp/checksum \
&& chmod +x /usr/local/bin/go-template /entrypoint.sh /usr/local/bin/cli \
&& chown -R 101:101 /etc/envoy

USER 101

# test configuration
RUN ENVOY_SERVICE_NAME=test-service JAEGER_AGENT_HOST=jaeger /entrypoint.sh \
/usr/local/bin/envoy \
-c /etc/envoy/envoy.yaml \
--service-cluster test \
--service-node test1-id \
--bootstrap-version 3 \
--log-level warn \
--mode validate

ENTRYPOINT [ "/entrypoint.sh" ]
CMD ["/usr/local/bin/envoy","-c","/etc/envoy/envoy.yaml"]
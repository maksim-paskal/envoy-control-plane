FROM envoyproxy/envoy:v1.21.0
# for decoding envoy critical exceptions - use image with debug symbols
# FROM paskalmaksim/envoy-docker-image:debug-base

ADD --chown=envoy:envoy https://github.com/maksim-paskal/go-template/releases/download/v0.1.0/go-template_0.1.0_linux_amd64 /usr/local/bin/go-template
ADD --chown=envoy:envoy https://github.com/maksim-paskal/jaeger-client-cpp/releases/download/v0.5.0/libjaegertracing_plugin.so /usr/local/lib/libjaegertracing_plugin.so

COPY ./cli /usr/local/bin/cli
COPY ./envoy/scripts /scripts
COPY ./envoy/entrypoint.sh /entrypoint.sh
COPY ./envoy/envoy.defaults /envoy.defaults/
COPY ./envoy/certs /certs/

RUN touch /tmp/checksum \
&& echo "473ce4db569c2adfb6fc52a35582310c66164a241b7d7c5cc514d334757b682a /usr/local/lib/libjaegertracing_plugin.so" >> /tmp/checksum \
&& echo "6eb0a7ba547f425c63887287e15448bb3e6d009b1c0750d93e07f08d6f1dad68 /usr/local/bin/go-template" >> /tmp/checksum \
&& cat /tmp/checksum \
&& sha256sum -c /tmp/checksum \
&& rm /tmp/checksum \
&& chmod +x /usr/local/bin/go-template /entrypoint.sh /usr/local/bin/cli /scripts/* \
&& chown -R 101:101 /etc/envoy \
&& apt update; apt install -y iptables \
&& apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false \
&& rm -rf /var/lib/apt/lists/*

USER 101

ENV XDS_CLUSTER_TYPE=STRICT_DNS
ENV XDS_CLUSTER_ADDRESS=envoy-control-plane

# test configuration
RUN ENVOY_SERVICE_NAME=test-service JAEGER_AGENT_HOST=jaeger /entrypoint.sh \
/usr/local/bin/envoy \
-c /etc/envoy/envoy.yaml \
--service-cluster test \
--service-node test1-id \
--log-level warn \
--mode validate && rm -rf /tmp/*

ENTRYPOINT [ "/entrypoint.sh" ]
CMD ["/usr/local/bin/envoy","-c","/etc/envoy/envoy.yaml","--log-level","warn"]
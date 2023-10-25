# for decoding envoy critical exceptions - use image with debug symbols
# paskalmaksim/envoy-docker-image:debug-base
ARG base=envoyproxy/envoy:v1.26.6

FROM $base
ARG TARGETARCH

ADD --chown=envoy:envoy https://github.com/maksim-paskal/go-template/releases/download/v0.1.2/go-template_0.1.2_linux_${TARGETARCH} /usr/local/bin/go-template
COPY --from=paskalmaksim/jaeger-client-cpp:0.5.0-clang --chown=envoy:envoy /libjaegertracing_plugin.so /usr/local/lib/libjaegertracing_plugin.so

COPY ./cli /usr/local/bin/cli
COPY ./envoy/scripts /scripts
COPY ./envoy/entrypoint.sh /entrypoint.sh
COPY ./envoy/envoy.defaults /envoy.defaults/
COPY ./envoy/certs /certs/

RUN apt update \
&& apt full-upgrade -y \
&& touch /tmp/checksum.amd64 \
&& echo "65d6b1e296dafb062c785c5a14135beeca953b11c577a70da74e5071793a4120 /usr/local/bin/go-template" >> /tmp/checksum.amd64 \
&& echo "473ce4db569c2adfb6fc52a35582310c66164a241b7d7c5cc514d334757b682a /usr/local/lib/libjaegertracing_plugin.so" >> /tmp/checksum.amd64 \
&& touch /tmp/checksum.arm64 \
&& echo "c70ad5472a7a4db5ee9fd2593ebbad1437f345c9ee4a0fda3ba688199350e277 /usr/local/bin/go-template" >> /tmp/checksum.arm64 \
&& echo "edb17cbd9e87afbb2f4612b37f52499ccbd9844d88c22144f0aafb129d3aabd7 /usr/local/lib/libjaegertracing_plugin.so" >> /tmp/checksum.arm64 \
&& sha256sum -c /tmp/checksum.${TARGETARCH} \
&& rm -rf /tmp/checksum* \
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
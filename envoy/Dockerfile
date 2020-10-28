
FROM envoyproxy/envoy:v1.15.2

ADD --chown=envoy:envoy https://github.com/maksim-paskal/go-template/releases/download/v0.0.6/go-template-linux-amd64 /usr/local/bin/go-template
ADD --chown=envoy:envoy https://github.com/maksim-paskal/jaeger-client-cpp/releases/download/v0.5.0/libjaegertracing_plugin.so /usr/local/lib/libjaegertracing_plugin.so
ADD --chown=envoy:envoy https://github.com/maksim-paskal/envoy-control-plane/releases/download/cli/cli-linux /usr/local/bin/cli

COPY ./entrypoint.sh /entrypoint.sh
COPY ./envoy.defaults /envoy.defaults/

RUN touch /tmp/checksum \
&& echo "473ce4db569c2adfb6fc52a35582310c66164a241b7d7c5cc514d334757b682a /usr/local/lib/libjaegertracing_plugin.so" >> /tmp/checksum \
&& echo "74de36b7de5a54f7d0b781cddd0a61062d2f27972f861efaf6bda0f4c801ee9c /usr/local/bin/go-template" >> /tmp/checksum \
&& echo "38c7da3317c05df8a4d9c17a9a517ff9363f8de5b5ad13b4685ce5e8c3d32525 /usr/local/bin/cli" >> /tmp/checksum \
&& cat /tmp/checksum \
&& sha256sum -c /tmp/checksum \
&& rm /tmp/checksum \
&& chmod +x /usr/local/bin/go-template /entrypoint.sh /usr/local/bin/cli \
&& chown -R 101:101 /etc/envoy

USER 101

ENTRYPOINT [ "/entrypoint.sh" ]
CMD ["/usr/local/bin/envoy","-c","/etc/envoy/envoy.yaml"]
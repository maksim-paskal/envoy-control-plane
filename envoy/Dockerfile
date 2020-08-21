
FROM envoyproxy/envoy:v1.15.0

ADD --chown=envoy:envoy https://github.com/maksim-paskal/go-template/releases/download/v0.0.6/go-template-linux-amd64 /usr/local/bin/go-template
ADD --chown=envoy:envoy https://github.com/maksim-paskal/jaeger-client-cpp/releases/download/v0.5.0/libjaegertracing_plugin.so /usr/local/lib/libjaegertracing_plugin.so
COPY ./entrypoint.sh /entrypoint.sh
COPY ./envoy.defaults /envoy.defaults/
RUN chmod +x /usr/local/bin/go-template /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
CMD ["/usr/local/bin/envoy","-c","/etc/envoy/envoy.yaml"]
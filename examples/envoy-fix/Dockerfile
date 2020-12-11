FROM envoyproxy/envoy-debug:v1.16.0

ADD https://raw.githubusercontent.com/envoyproxy/envoy/master/tools/stack_decode.py /stack_decode.py

RUN apt update \
&& apt install -y python3 binutils \
&& chmod 777 /stack_decode.py
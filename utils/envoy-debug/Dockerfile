FROM envoyproxy/envoy-debug:v1.26.7

ADD https://raw.githubusercontent.com/envoyproxy/envoy/master/tools/stack_decode.py /stack_decode.py

RUN apt update \
&& apt install -y python3 binutils gdb \
&& chmod 777 /stack_decode.py \
&& rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*
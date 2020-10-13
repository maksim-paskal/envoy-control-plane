FROM ubuntu:bionic

RUN apt update \
&& apt install -y build-essential git curl libc++-dev libc++abi-dev clang-9 \
&& curl -L -O "https://cmake.org/files/v3.11/cmake-3.11.0-Linux-x86_64.sh" \
&& bash cmake-3.11.0-Linux-x86_64.sh --skip-license

RUN echo 'INPUT(-l:libc++.a -l:libc++abi.a -lm -lpthread)' > /usr/lib/x86_64-linux-gnu/libc++.so

WORKDIR /tmp
RUN git clone --recurse-submodules --branch=v0.5.0 https://github.com/jaegertracing/jaeger-client-cpp.git

WORKDIR /tmp/jaeger-client-cpp/
ADD ./jaeger-client-cpp-build.patch ./jaeger-client-cpp-build.patch
RUN git apply jaeger-client-cpp-build.patch
RUN ./scripts/build-plugin.sh

# /tmp/jaeger-client-cpp/build/libjaegertracing_plugin.so
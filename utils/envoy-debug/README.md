# How to debug Envoy exceptions

## 1. Coredumps

It's most efficient way to debug an exception - to enable it - you need to

### 1a. Prepare env

```bash
# update soft limits on the system
ulimit -S -c unlimited

# locate path for core dumps
sysctl -w kernel.core_pattern='/envoy/core-%e.%p.%h.%t'
```

### 1b. Run envoy with debug symbols

You need run envoy with debug symbols - for example docker image with debug sympols `envoyproxy/envoy-debug:<envoy-version>`

after next exception linux will create coredump

## 2. For example envoy return exception trace

```log
[2022-02-14 23:28:53.199][22][critical][main] [source/exe/terminate_handler.cc:12] std::terminate called! (possible uncaught exception, see trace)
[2022-02-14 23:28:53.199][22][critical][backtrace] [./source/server/backtrace.h:91] Backtrace (use tools/stack_decode.py to get line numbers):
[2022-02-14 23:28:53.199][22][critical][backtrace] [./source/server/backtrace.h:92] Envoy version: a9d72603c68da3a10a1c0d021d01c7877e6f2a30/1.21.0/Clean/RELEASE/BoringSSL
[2022-02-14 23:28:53.218][22][critical][backtrace] [./source/server/backtrace.h:96] #0: Envoy::TerminateHandler::logOnTerminate()::$_0::operator()() [0x55d953a74f0e]
[2022-02-14 23:28:53.228][22][critical][backtrace] [./source/server/backtrace.h:98] #1: [0x55d953a74dd9]
[2022-02-14 23:28:53.237][22][critical][backtrace] [./source/server/backtrace.h:96] #2: std::__terminate() [0x55d953f25433]
[2022-02-14 23:28:53.246][22][critical][backtrace] [./source/server/backtrace.h:96] #3: std::__1::__function::__func<>::operator()() [0x55d9535f67a3]
[2022-02-14 23:28:53.258][22][critical][backtrace] [./source/server/backtrace.h:96] #4: event_process_active_single_queue [0x55d953915220]
[2022-02-14 23:28:53.268][22][critical][backtrace] [./source/server/backtrace.h:96] #5: event_base_loop [0x55d953913f11]
[2022-02-14 23:28:53.278][22][critical][backtrace] [./source/server/backtrace.h:96] #6: Envoy::Server::InstanceImpl::run() [0x55d95318261c]
[2022-02-14 23:28:53.287][22][critical][backtrace] [./source/server/backtrace.h:96] #7: Envoy::MainCommonBase::run() [0x55d951d5cd64]
[2022-02-14 23:28:53.297][22][critical][backtrace] [./source/server/backtrace.h:96] #8: Envoy::MainCommon::main() [0x55d951d5d5d6]
[2022-02-14 23:28:53.307][22][critical][backtrace] [./source/server/backtrace.h:96] #9: main [0x55d951d5979c]
[2022-02-14 23:28:53.310][22][critical][backtrace] [./source/server/backtrace.h:96] #10: __libc_start_main [0x7f4eda35dbf7]
[2022-02-14 23:28:53.310][22][critical][backtrace] [./source/server/backtrace.h:104] Caught Aborted, suspect faulting address 0x6500000016
[2022-02-14 23:28:53.310][22][critical][backtrace] [./source/server/backtrace.h:91] Backtrace (use tools/stack_decode.py to get line numbers):
[2022-02-14 23:28:53.310][22][critical][backtrace] [./source/server/backtrace.h:92] Envoy version: a9d72603c68da3a10a1c0d021d01c7877e6f2a30/1.21.0/Clean/RELEASE/BoringSSL
[2022-02-14 23:28:53.310][22][critical][backtrace] [./source/server/backtrace.h:96] #0: __restore_rt [0x7f4eda73f980]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:98] #1: [0x55d953a74dd9]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #2: std::__terminate() [0x55d953f25433]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #3: std::__1::__function::__func<>::operator()() [0x55d9535f67a3]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #4: event_process_active_single_queue [0x55d953915220]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #5: event_base_loop [0x55d953913f11]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #6: Envoy::Server::InstanceImpl::run() [0x55d95318261c]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #7: Envoy::MainCommonBase::run() [0x55d951d5cd64]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #8: Envoy::MainCommon::main() [0x55d951d5d5d6]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #9: main [0x55d951d5979c]
[2022-02-14 23:28:53.320][22][critical][backtrace] [./source/server/backtrace.h:96] #10: __libc_start_main [0x7f4eda35dbf7]
```

### 2a. Find the static address of the entry of Envoy::MainCommon::main()

```bash
objdump -Cd /usr/local/bin/envoy | fgrep <main> -A 20 
```

for example

```log
fef797: e8 84 3d 00 00 callq ff3520 <Envoy::MainCommon::main(int, char**, std::__1::function<void (Envoy::Server::Instance&)>)>
```

static address of `Envoy::MainCommon::main()` will be `fef797` = `0xfef797`

### 2b. Compute the static address of exception 0x55d9535f67a3

we also need `main` address from exception trace = `0x55d951d5979c` and the static address of the entry of Envoy::MainCommon::main() = `0xfef797`

```bash
python3 -c 'print(hex(0x55d9535f67a3-0x55d951d5979c+0xfef797))'
# result: 0x288c79e
```

### 2c. Use addr2line get the line of the code

```bash
addr2line -Ce /usr/local/bin/envoy 0x288c79e
# result: /proc/self/cwd/source/common/config/ttl.cc:30
```

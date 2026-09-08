#!/bin/sh
# Yearning 容器入口：后台拉起审核引擎（gRPC），再把主程序作为前台进程 exec。
# 说明：
#   - 引擎监听 127.0.0.1:13307，与 /opt/conf.toml 中 [General].RpcAddr 保持一致。
#   - 主程序只会在真正发起 Engine.* 调用时才连接引擎，故此处不额外做就绪等待。
#   - 通过 docker 的 command 覆盖可执行 install/migrate/reset_super（此时引擎已后台就绪，无害）。

/opt/engine -addr 127.0.0.1:13307 &

exec "$@"

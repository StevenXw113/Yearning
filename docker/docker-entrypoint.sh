#!/bin/sh
# 保留脚本是为了能直接覆盖 CMD，如：docker compose run --rm yearning /opt/Yearning install
exec "$@"

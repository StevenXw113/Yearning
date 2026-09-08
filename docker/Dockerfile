# =====================================================================
# Yearning —— 基于源代码编译的一体化镜像（前端 + Go 主程序 + 审核引擎）
#
# 构建（上下文为仓库根目录，即从 c:/hub/Yearning 下执行）：
#   docker build -f docker/Dockerfile -t yearning:dev .
# 国内网络默认已启用加速（见下方 ARG，构建时可覆盖）：
#   docker build --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
#                --build-arg GOPROXY_URL=https://goproxy.cn,direct \
#                -f docker/Dockerfile -t yearning:dev .
# =====================================================================

# ---------- 阶段一：前端构建（Vue3 + Vite）----------
FROM node:20-alpine AS frontend
ARG NPM_REGISTRY=https://registry.npmmirror.com
WORKDIR /src/front

# 仅拷贝清单与锁文件，先装依赖以命中 Docker 层缓存
COPY front/package.json front/yarn.lock front/.npmrc ./
RUN yarn config set registry "$NPM_REGISTRY" \
 && yarn install --network-timeout 600000

# 再拷贝源码并构建（产物输出到 /src/front/dist）
COPY front/ ./
RUN yarn build

# 供后端 go:embed 使用的 chat 占位页（chat 子工程为独立源码，未纳入本镜像）
RUN mkdir -p /chat/server/app \
 && printf '<!DOCTYPE html><html><head><title>Y chat</title></head><body><div id="app"></div></body></html>' > /chat/server/app/index.html

# ---------- 阶段二：Go 后端主程序 + 审核引擎 编译 ----------
FROM golang:1.26 AS build
ARG GOPROXY_URL=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY_URL \
    CGO_ENABLED=0

WORKDIR /src

# 主 module 依赖（先装依赖，命中层缓存）
COPY go.mod go.sum ./
COPY engine/go.mod engine/go.sum ./engine/
RUN go mod download

# 拷贝 Go 相关源码（前端由阶段一提供 dist，不入此阶段）
COPY main.go ./
COPY cmd ./cmd
COPY src ./src
COPY engine ./engine

# 准备 go:embed 所需的两个目录
COPY --from=frontend /src/front/dist ./src/service/dist/
COPY --from=frontend /chat/server/app/index.html ./src/service/chat/server/app/index.html

# 编译主程序（二进制命名沿用既有约定 Yearning）
RUN go build -trimpath -ldflags "-s -w" -o /out/Yearning .

# 编译独立的审核引擎（gRPC 服务，源码在 engine/，独立 module）
WORKDIR /src/engine
RUN go build -trimpath -ldflags "-s -w" -o /out/engine ./cmd/engine

# ---------- 阶段三：运行时 ----------
FROM alpine:3.20.2 AS run

# 国内 apk 源 + 运行期依赖（时区、CA、bash）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
 && apk update \
 && apk add --no-cache ca-certificates bash tzdata libc6-compat dumb-init \
 && cp -rf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
 && echo "Asia/Shanghai" > /etc/timezone

COPY --from=build /out/Yearning /opt/Yearning
COPY --from=build /out/engine   /opt/engine
COPY docker/docker-entrypoint.sh /opt/docker-entrypoint.sh
COPY docker/conf.toml /opt/conf.toml

RUN chmod +x /opt/docker-entrypoint.sh

WORKDIR /opt
ENV IS_DOCKER=is_docker
EXPOSE 8000 13307

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/opt/docker-entrypoint.sh"]
CMD ["/opt/Yearning", "run"]



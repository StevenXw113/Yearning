# Yearning 后端镜像（纯 API）：docker build -f docker/Dockerfile -t yearning:dev .
# 上下文为仓库根目录；前端见 docker/frontend.Dockerfile，引擎见 engine/Dockerfile

# ---------- 阶段一：Go 后端编译 ----------
FROM golang:1.26 AS build
ARG GOPROXY_URL=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY_URL \
    CGO_ENABLED=0

WORKDIR /src

# 先装依赖，命中层缓存
COPY go.mod go.sum ./
COPY engine/go.mod engine/go.sum ./engine/
RUN go mod download

COPY main.go ./
COPY cmd ./cmd
COPY src ./src
# engine/ 源码是 go.mod replace 的目标，主程序编译需要
COPY engine ./engine

# go:embed 要求目录非空，只放占位页（真实前端在 frontend 镜像里）
RUN mkdir -p src/service/dist \
 && printf '%s' '<!DOCTYPE html><html><head><meta charset="utf-8"><title>Yearning</title></head><body><p>Yearning API 服务运行中。前端页面请访问前端容器。</p></body></html>' > src/service/dist/index.html

RUN go build -trimpath -ldflags "-s -w" -o /out/Yearning .

# gh-ost：OSC（OscMinRows>0 时启用）的运行期依赖
ARG GH_OST_VERSION=v1.1.11
RUN GOBIN=/out go install github.com/github/gh-ost/go/cmd/gh-ost@${GH_OST_VERSION}

# ---------- 阶段二：运行时 ----------
FROM alpine:3.20.2 AS run

# 国内 apk 源 + 运行期依赖
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
 && apk update \
 && apk add --no-cache ca-certificates bash tzdata libc6-compat dumb-init \
 && cp -rf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
 && echo "Asia/Shanghai" > /etc/timezone

COPY --from=build /out/Yearning /opt/Yearning
COPY --from=build /out/gh-ost /usr/local/bin/gh-ost
COPY docker/docker-entrypoint.sh /opt/docker-entrypoint.sh
COPY docker/conf.toml /opt/conf.toml

RUN chmod +x /opt/docker-entrypoint.sh

WORKDIR /opt
ENV IS_DOCKER=is_docker
EXPOSE 8000

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/opt/docker-entrypoint.sh"]
CMD ["/opt/Yearning", "run"]

# Yearning 审核引擎（独立服务）
# docker build -f engine/Dockerfile -t yearning-engine:1.0.0 ./engine   （上下文为 engine/ 目录）
# 运行时为 distroless：无 shell，无 curl，只能看日志排查

# build stage
FROM golang:1.26 AS build
ARG GOPROXY_URL=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY_URL \
    CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/engine ./cmd/engine

# run stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/engine /engine
EXPOSE 13307
ENTRYPOINT ["/engine"]
CMD ["-addr", ":13307"]

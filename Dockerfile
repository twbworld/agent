##编译
FROM golang:1.25 AS builder
WORKDIR /app
ARG TARGETARCH
ENV GO111MODULE=on
# ENV GOPROXY="https://goproxy.cn,direct"
COPY go.mod go.sum ./
RUN go mod download
COPY . .
#请在CI/CD中使用"git describe --tags --always"获取版本号并设置VERSION参数
ARG VERSION="unknown"
#chroma-go/go-sqlite3需要cgo编译; 且使用完全静态编译, 否则需依赖外部安装的glibc
RUN CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH go build -tags 'netgo osusergo' -ldflags "-s -w --extldflags '-static -fpic' -X 'gitee.com/taoJie_1/mall-agent/global.Version=${VERSION}'" -o server . && \
    mv config.example.yaml server /app/static


##打包镜像
FROM alpine:latest
LABEL org.opencontainers.image.vendor="淘街"
LABEL org.opencontainers.image.authors="1174865138@qq.com"
LABEL org.opencontainers.image.description="AI调度服务"
LABEL org.opencontainers.image.source="https://gitee.com/taoJie_1/mall-agent"
WORKDIR /app
COPY --from=builder /app/static/ static/
RUN set -xe && \
    mv static/config.example.yaml config.yaml && \
    mv static/server server && \
    chmod +x server && \
    # sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && \
    apk add -U --no-cache tzdata ca-certificates && \
    apk cache clean && \
    rm -rf /var/cache/apk/*
# EXPOSE 80
ENTRYPOINT ["./server"]

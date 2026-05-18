FROM tbj7-xtiao-tcr1.tencentcloudcr.com/xtiao-release/golang:1.26 as build

ENV GOPROXY https://goproxy.cn,direct
ENV GO111MODULE on

WORKDIR /go/cache
ADD go.mod .
ADD go.sum .
RUN go mod download

WORKDIR /go/release
ADD . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o app ./main.go

FROM tbj7-xtiao-tcr1.tencentcloudcr.com/xtiao-release/alpine:3.21 as prod
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
RUN mkdir -p /usr/share/zoneinfo/Asia && \
    ln -s /etc/localtime /usr/share/zoneinfo/Asia/Shanghai
RUN echo "Asia/Shanghai" > /etc/timezone
ENV TZ=Asia/Shanghai

WORKDIR /home
COPY --from=build /go/release/app /home/app
COPY --from=build /go/release/components.json /home/components.json

ENTRYPOINT ["/home/app"]

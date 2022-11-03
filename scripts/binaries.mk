define DOCKERFILE_BINARIES
FROM $(RPI32_IMAGE) AS rpicamera32
RUN ["cross-build-start"]
RUN apt update && apt install -y g++ pkg-config make libcamera-dev
WORKDIR /s/internal/rpicamera
COPY internal/rpicamera .
RUN cd exe && make -j$$(nproc)

FROM $(RPI64_IMAGE) AS rpicamera64
RUN ["cross-build-start"]
RUN apt update && apt install -y g++ pkg-config make libcamera-dev
WORKDIR /s/internal/rpicamera
COPY internal/rpicamera .
RUN cd exe && make -j$$(nproc)

FROM $(BASE_IMAGE)
RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

ENV VERSION $(shell git describe --tags)
ENV CGO_ENABLED 0
RUN mkdir tmp binaries
RUN cp rtsp-simple-server.yml LICENSE tmp/

RUN GOOS=windows GOARCH=amd64 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server.exe
RUN cd tmp && zip -q ../binaries/rtsp-simple-server_$${VERSION}_windows_amd64.zip rtsp-simple-server.exe rtsp-simple-server.yml LICENSE

RUN GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server
RUN tar -C tmp -czf binaries/rtsp-simple-server_$${VERSION}_linux_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml LICENSE

RUN GOOS=darwin GOARCH=amd64 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server
RUN tar -C tmp -czf binaries/rtsp-simple-server_$${VERSION}_darwin_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml LICENSE

COPY --from=rpicamera32 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server -tags rpicamera
RUN tar -C tmp -czf binaries/rtsp-simple-server_$${VERSION}_linux_armv6.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml LICENSE
RUN rm internal/rpicamera/exe/exe

COPY --from=rpicamera32 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server -tags rpicamera
RUN tar -C tmp -czf binaries/rtsp-simple-server_$${VERSION}_linux_armv7.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml LICENSE
RUN rm internal/rpicamera/exe/exe

COPY --from=rpicamera64 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm64 go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o tmp/rtsp-simple-server -tags rpicamera
RUN tar -C tmp -czf binaries/rtsp-simple-server_$${VERSION}_linux_arm64v8.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml LICENSE
RUN rm internal/rpicamera/exe/exe
endef
export DOCKERFILE_BINARIES

binaries:
	echo "$$DOCKERFILE_BINARIES" | DOCKER_BUILDKIT=1 docker build . -f - -t temp
	docker run --rm -v $(PWD):/out \
	temp sh -c "rm -rf /out/binaries && cp -r /s/binaries /out/"

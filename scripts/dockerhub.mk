define DOCKERFILE_DOCKERHUB
FROM scratch
ARG BINARY
ADD $$BINARY /
ENTRYPOINT [ "/rtsp-simple-server" ]
endef
export DOCKERFILE_DOCKERHUB

define DOCKERFILE_DOCKERHUB_RPI_32
FROM $(RPI32_IMAGE) AS base
RUN apt update && apt install -y --no-install-recommends libcamera0
ARG BINARY
ADD $$BINARY /
ENTRYPOINT [ "/rtsp-simple-server" ]
endef
export DOCKERFILE_DOCKERHUB_RPI_32

define DOCKERFILE_DOCKERHUB_RPI_64
FROM $(RPI64_IMAGE)
RUN apt update && apt install -y --no-install-recommends libcamera0
ARG BINARY
ADD $$BINARY /
ENTRYPOINT [ "/rtsp-simple-server" ]
endef
export DOCKERFILE_DOCKERHUB_RPI_64

dockerhub:
	$(eval export DOCKER_CLI_EXPERIMENTAL=enabled)
	$(eval VERSION := $(shell git describe --tags))

	docker login -u $(DOCKER_USER) -p $(DOCKER_PASSWORD)

	docker buildx rm builder 2>/dev/null || true
	rm -rf $$HOME/.docker/manifests/*
	docker buildx create --name=builder --use

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/amd64 \
	--build-arg BINARY="$$(echo binaries/*linux_amd64.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-amd64 \
	-t aler9/rtsp-simple-server:latest-amd64  \
	--push

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v6 \
	--build-arg BINARY="$$(echo binaries/*linux_armv6.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-armv6 \
	-t aler9/rtsp-simple-server:latest-armv6 \
	--push

	echo "$$DOCKERFILE_DOCKERHUB_RPI_32" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v6 \
	--build-arg BINARY="$$(echo binaries/*linux_armv6.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-armv6-rpi \
	-t aler9/rtsp-simple-server:latest-armv6-rpi \
	--push

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v7 \
	--build-arg BINARY="$$(echo binaries/*linux_armv7.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-armv7 \
	-t aler9/rtsp-simple-server:latest-armv7 \
	--push

	echo "$$DOCKERFILE_DOCKERHUB_RPI_32" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v7 \
	--build-arg BINARY="$$(echo binaries/*linux_armv7.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-armv7-rpi \
	-t aler9/rtsp-simple-server:latest-armv7-rpi \
	--push

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm64/v8 \
	--build-arg BINARY="$$(echo binaries/*linux_arm64v8.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-arm64v8 \
	-t aler9/rtsp-simple-server:latest-arm64v8 \
	--push

	echo "$$DOCKERFILE_DOCKERHUB_RPI_64" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm64/v8 \
	--build-arg BINARY="$$(echo binaries/*linux_arm64v8.tar.gz)" \
	-t aler9/rtsp-simple-server:$(VERSION)-arm64v8-rpi \
	-t aler9/rtsp-simple-server:latest-arm64v8-rpi \
	--push

	docker manifest create aler9/rtsp-simple-server:$(VERSION)-rpi \
	$(foreach ARCH,armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH)-rpi)
	docker manifest push aler9/rtsp-simple-server:$(VERSION)-rpi

	docker manifest create aler9/rtsp-simple-server:$(VERSION) \
	$(foreach ARCH,amd64 armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:$(VERSION)

	docker manifest create aler9/rtsp-simple-server:latest-rpi \
	$(foreach ARCH,armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH)-rpi)
	docker manifest push aler9/rtsp-simple-server:latest-rpi

	docker manifest create aler9/rtsp-simple-server:latest \
	$(foreach ARCH,amd64 armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:latest

	docker buildx rm builder
	rm -rf $$HOME/.docker/manifests/*

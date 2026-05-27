.PHONY: build docker-build clean

APP_NAME  := runc-rootfs-persist
IMG_NAME  := runc-rootfs-persist:latest

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(APP_NAME) ./cmd/wrapper/

docker-build:
	docker build -t $(IMG_NAME) .

clean:
	rm -rf bin/

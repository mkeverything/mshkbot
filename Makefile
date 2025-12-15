export
-include .env

BINARY_NAME=bin/mshkbot
LDFLAGS=

# download dependencies
.PHONY: deps
deps:
	go mod download

# build binary
.PHONY: build
build: deps
	CGO_ENABLED=0 GOOS=linux go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/mshkbot

# build docker image
.PHONY: docker-build
docker-build: build
	docker buildx build -t sukalov/mshkbot --platform linux/amd64 .

# push docker image
.PHONY: docker-push
docker-push:
	docker push sukalov/mshkbot:latest

# development run with Air hot reload
.PHONY: dev
dev:
	air

# clean up old docker images
.PHONY: docker-clean
docker-clean:
	ssh root@${DEPLOY_HOST} "\
		docker stop mshk || true; \
		docker rm mshk || true; \
		docker image prune -f \
	"

# deployment command
.PHONY: deploy
deploy: build docker-build docker-push docker-clean
	ssh root@${DEPLOY_HOST} "\
		cd /root && \
		docker pull sukalov/mshkbot:latest && \
		docker run --name mshk \
		--restart always \
		-v /root/mshk/.env:/root/.env:ro \
		-d sukalov/mshkbot:latest \
	"
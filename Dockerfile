# syntax=docker/dockerfile:1

FROM docker.io/library/golang:1.25 AS build

WORKDIR /workdir

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -o /pod-image-policy .

FROM scratch

COPY --from=build /pod-image-policy /pod-image-policy

EXPOSE 9443

ENTRYPOINT ["/pod-image-policy"]

FROM docker.io/library/golang:1.25 AS build

WORKDIR /workdir

COPY . .

RUN CGO_ENABLED=0 go build -o /pod-image-admission .

FROM scratch

COPY --from=build /pod-image-admission /pod-image-admission

EXPOSE 9443

ENTRYPOINT ["/pod-image-admission"]

FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
        -o /out/linktheca-server ./cmd/linktheca-server

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/linktheca-server /usr/local/bin/linktheca-server

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/linktheca-server"]

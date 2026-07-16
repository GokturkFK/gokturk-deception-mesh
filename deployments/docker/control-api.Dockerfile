# syntax=docker/dockerfile:1
# Build context: repo koku (bkz. deployments/docker/docker-compose.yml)

FROM golang:1.22-alpine AS build
WORKDIR /src
# go.sum henuz yok (bagimlilik eklenince olusacak); glob sayesinde varsa kopyalanir.
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-api ./cmd/control-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/control-api /control-api
USER nonroot:nonroot
EXPOSE 8080
# distroless'ta wget/curl yok; ayni binary healthcheck modunda calisiyor.
HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=5 \
  CMD ["/control-api", "healthcheck"]
ENTRYPOINT ["/control-api"]

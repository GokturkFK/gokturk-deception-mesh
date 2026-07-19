# syntax=docker/dockerfile:1
# sensor-ssh: auth.log tailer + SSH parola parseri (APP-4) + Decode/NATS
# publish (APP-5, @fetihcakmak). HTTP sunucusu yok (yalnizca log tailer +
# NATS publisher + control-api'ye periyodik canary sorgusu), bu yuzden
# control-api'nin aksine bir HEALTHCHECK subcommand'i yok; docker-compose.yml
# hicbir servis sensor-ssh'in "healthy" olmasina bagimli degil.
# Build context: repo koku (bkz. deployments/docker/docker-compose.yml)

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sensor-ssh ./cmd/sensor-ssh

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sensor-ssh /sensor-ssh
USER nonroot:nonroot
ENTRYPOINT ["/sensor-ssh"]

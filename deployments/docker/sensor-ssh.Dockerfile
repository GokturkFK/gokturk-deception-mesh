# syntax=docker/dockerfile:1
# TASLAK — cmd/sensor-ssh henuz yazilmadi (APP-4/5, @fetihcakmak). Bu Dockerfile
# derlenemez/test edilemez; kod gelince Cyber ile birlikte gozden gecirilecek
# (ozellikle HEALTHCHECK: HTTP sunucusu olmayan bir log-tailer/NATS publisher
# icin anlamli bir saglik sinyali APP-4/5'in ic mimarisine bagli).
# Build context: repo koku (bkz. deployments/docker/docker-compose.yml)

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sensor-ssh ./cmd/sensor-ssh

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sensor-ssh /sensor-ssh
USER nonroot:nonroot
ENTRYPOINT ["/sensor-ssh"]

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api-gateway ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/api-gateway /usr/local/bin/api-gateway
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api-gateway"]

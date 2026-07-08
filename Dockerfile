FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git build-base pkgconf libwebp-dev

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/speedrun ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata libwebp
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=build /out/speedrun /usr/local/bin/speedrun

RUN mkdir -p /app/data/uploads && chown -R app:app /app

USER app

ENV APP_ENV=production
ENV PORT=3000
ENV UPLOAD_DIR=/app/data/uploads
ENV AUTH_COOKIE_SECURE=true
ENV ALLOW_DEV_MOCK_AUTH=false

EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/speedrun"]

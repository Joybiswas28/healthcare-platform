# syntax=docker/dockerfile:1

# ---- build
FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/createadmin ./cmd/createadmin \
 && mkdir -p /out/data/uploads

# ---- run: no shell, no package manager, runs as an unprivileged user
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /out/createadmin /app/
# Owned by the nonroot user so a fresh volume mounted here is writable.
COPY --from=build --chown=65532:65532 /out/data /data
USER nonroot:nonroot
ENV APP_ENV=production HTTP_ADDR=:8080 UPLOAD_DIR=/data/uploads
EXPOSE 8080
ENTRYPOINT ["/app/api"]

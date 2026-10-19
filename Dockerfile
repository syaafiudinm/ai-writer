# syntax=docker/dockerfile:1
# Dijalankan dari root repo:  docker build -t ai-writer .
# Railway otomatis memakai file ini.

FROM node:22-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# distroless/static sudah berisi sertifikat CA (untuk HTTPS ke Gemini);
# data zona waktu ikut di binary lewat time/tzdata.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend /out/server /out/seed /app/
COPY --from=frontend /src/dist /app/web
ENV PORT=8080 \
    STATIC_DIR=/app/web
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]

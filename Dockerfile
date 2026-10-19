# syntax=docker/dockerfile:1
# Dijalankan dari root repo:  docker build -t ai-writer .
# Railway otomatis memakai file ini. Tahap build React ditambahkan setelah frontend dibuat (M5).

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
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]

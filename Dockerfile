# syntax=docker/dockerfile:1

# Il frontend e' identico per ogni architettura: lo costruiamo sempre in
# modo nativo sulla piattaforma di build, senza emulazione.
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Anche il backend gira nativo: essendo CGO-free, cross-compila per la
# piattaforma di destinazione tramite GOOS/GOARCH.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend-build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-build /app/backend/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=backend-build /out/server /server
EXPOSE 8080
ENV DATA_DIR=/data
ENTRYPOINT ["/server"]

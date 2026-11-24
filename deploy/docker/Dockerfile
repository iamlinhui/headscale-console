FROM node:22-alpine AS frontend
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend ./frontend
RUN pnpm build

FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY cmd ./cmd
COPY main.go ./
COPY --from=frontend /app/internal/webassets/static/dist ./internal/webassets/static/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /console . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /hash-password ./cmd/hash-password

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
LABEL org.opencontainers.image.title="Headscale Console" \
      org.opencontainers.image.description="Vue 3 + Naive UI and Go management console for Headscale"
ENV APP_ENV=production HOST=0.0.0.0 PORT=3080
COPY --from=backend /console /app/console
COPY --from=backend /hash-password /app/hash-password
EXPOSE 3080
ENTRYPOINT ["/app/console"]

# grapi, pure Go build (no libvips yet: see docs/divergences.md, Médias).
# Build on the homelab: make docker.
FROM oven/bun:1.2.20 AS admin
WORKDIR /src/admin
COPY admin/package.json admin/bun.lock ./
RUN bun install --frozen-lockfile
COPY admin/ ./
RUN bun run build

FROM golang:1.26-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=admin /src/admin/dist ./admin/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -tags adminui -ldflags "-s -w -X main.version=${VERSION}" -o /out/grapi ./cmd/grapi

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --uid 1000 --create-home --home-dir /app grapi
COPY --from=build /out/grapi /usr/local/bin/grapi
WORKDIR /app
USER 1000
ENV HOST=0.0.0.0 PORT=1337
EXPOSE 1337
VOLUME ["/app/public/uploads", "/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD ["grapi", "health"]
ENTRYPOINT ["grapi"]
CMD ["start", "-dir", "/app"]

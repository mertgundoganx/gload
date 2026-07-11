# Build stage — runs on the builder's native arch and cross-compiles to the
# target arch (fast, no emulation) since the binary is pure Go (CGO disabled).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o gload .

# Runtime stage
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

# Run as an unprivileged user. The app stores its SQLite DB under $HOME/.gload,
# so HOME must point at the user's home for DefaultDBPath to resolve.
RUN adduser -D -u 10001 -h /home/app app
ENV HOME=/home/app

WORKDIR /app
COPY --from=builder /app/gload .
RUN mkdir -p /home/app/.gload && chown -R app:app /home/app /app
USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./gload"]
CMD ["--web", "--port", "8080"]

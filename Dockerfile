FROM --platform=${BUILDPLATFORM} golang:1.19-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
# Git is required for getting the dependencies.
# hadolint ignore=DL3018
RUN apk add --no-cache git

WORKDIR /src

# Fetch dependencies first; they are less susceptible to change on every build
# and will therefore be cached for speeding up the next build
COPY ./go.mod ./go.sum ./
RUN go mod download
# Install the swag tool that generates swagger docs from the source code
RUN go install github.com/swaggo/swag/cmd/swag@v1.8.12

# Import the code from the context.
COPY ./ ./
# Generate API documentation
RUN CGO_ENABLED=0 swag init --parseDependency true

# Build the executable to `/app`. Mark the build as statically linked.
# hadolint ignore=SC2155
RUN export TAG=$(git describe --tags "$(git rev-list --tags --max-count=1)") && \
    export COMMIT=$(git rev-parse --short HEAD) && \
    CGO_ENABLED=0 \
    # thanks kurokobo!
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -installsuffix 'static' \
    -ldflags="-X main.version=${TAG} -X main.commit=${COMMIT}" \
    -o /app .

FROM alpine:3.13.0 AS final

# Set up non-root user and app directory
# * Non-root because of the principle of least privlege
# * App directory to allow mounting volumes
RUN addgroup -g 1000 bot && \
    adduser -HD -u 1000 -G bot bot && \
    mkdir -p /app/logs /app/locales /app/storage && \
    chown -R bot:bot /app
USER bot
WORKDIR /app

# Import the compiled executable and locales.
COPY --from=builder /app /app
COPY ./locales/ /app/locales
COPY ./storage/postgres.sql /app/storage/postgres.sql

# Port used for AMU API
EXPOSE 5000
# Port used for health/liveliness checks
EXPOSE 8080
# Port used for prometheus metrics
EXPOSE 2112

ENV LOCALE_PATH="/app/locales" \
    LOG_PATH="/app/logs"
VOLUME ["/app/logs"]

# Run the compiled binary.
ENTRYPOINT ["./app"]

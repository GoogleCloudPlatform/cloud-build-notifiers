
FROM golang AS build-env
COPY . /go-src/
WORKDIR /go-src/
RUN go build -o /go-app .

# From the Cloud Run docs:
# https://cloud.google.com/run/docs/tutorials/pubsub#looking_at_the_code
# Use the official Debian slim image for a lean production container.
# https://hub.docker.com/_/debian
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

FROM gcr.io/distroless/base
COPY --from=build-env /go-app /
ENTRYPOINT ["/go-app", "--v=1", "--alsologtostderr"]
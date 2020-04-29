# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang AS build-env
COPY . /go-src/
WORKDIR /go-src/smtp
RUN go test /go-src/smtp
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
ENTRYPOINT ["/go-app", "--alsologtostderr", "--v=0"]

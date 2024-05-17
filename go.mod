module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.21

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.112.2
	cloud.google.com/go/bigquery v1.60.0
	cloud.google.com/go/cloudbuild v1.16.0
	cloud.google.com/go/secretmanager v1.12.0
	cloud.google.com/go/storage v1.40.0
	github.com/golang/glog v1.2.1
	github.com/google/cel-go v0.20.1
	github.com/google/go-cmp v0.6.0
	github.com/google/go-containerregistry v0.19.1
	github.com/slack-go/slack v0.12.5
	google.golang.org/api v0.174.0
	google.golang.org/protobuf v1.33.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/client-go v0.29.4
)

require (
	cloud.google.com/go/auth v0.2.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.0 // indirect
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	cloud.google.com/go/iam v1.1.7 // indirect
	cloud.google.com/go/longrunning v0.5.6 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/apache/arrow/go/v14 v14.0.2 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.15.1 // indirect
	github.com/docker/cli v26.0.1+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker v26.0.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/flatbuffers v24.3.25+incompatible // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.3 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.50.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.50.0 // indirect
	go.opentelemetry.io/otel v1.25.0 // indirect
	go.opentelemetry.io/otel/metric v1.25.0 // indirect
	go.opentelemetry.io/otel/trace v1.25.0 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/exp v0.0.0-20240416160154-fe59bbe5cc7f // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/oauth2 v0.19.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.20.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	google.golang.org/genproto v0.0.0-20240415180920-8c6c420018be // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240415180920-8c6c420018be // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240415180920-8c6c420018be // indirect
	google.golang.org/grpc v1.63.2 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)

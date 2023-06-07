module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.16

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.110.0
	cloud.google.com/go/bigquery v1.51.0
	cloud.google.com/go/cloudbuild v1.10.0
	cloud.google.com/go/secretmanager v1.10.0
	cloud.google.com/go/storage v1.30.1
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/apache/thrift v0.18.1 // indirect
	github.com/docker/cli v23.0.4+incompatible // indirect
	github.com/docker/docker v23.0.4+incompatible // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/golang/glog v1.1.1
	github.com/golang/protobuf v1.5.3
	github.com/google/cel-go v0.14.0
	github.com/google/flatbuffers v23.3.3+incompatible // indirect
	github.com/google/go-cmp v0.5.9
	github.com/google/go-containerregistry v0.14.0
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/slack-go/slack v0.12.2
	github.com/stoewer/go-strcase v1.3.0 // indirect
	golang.org/x/exp v0.0.0-20230420155640-133eef4313cb // indirect
	golang.org/x/tools v0.8.0 // indirect
	google.golang.org/api v0.125.0
	google.golang.org/protobuf v1.30.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/client-go v0.27.1
)

module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.16

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.110.0
	cloud.google.com/go/bigquery v1.50.0
	cloud.google.com/go/cloudbuild v1.10.0
	cloud.google.com/go/secretmanager v1.11.0
	cloud.google.com/go/storage v1.29.0
	github.com/antlr/antlr4 v0.0.0-20210404160547-4dfacf63e228 // indirect
	github.com/docker/cli v20.10.5+incompatible // indirect
	github.com/docker/docker v20.10.5+incompatible // indirect
	github.com/golang/glog v1.1.0
	github.com/golang/protobuf v1.5.3
	github.com/google/cel-go v0.7.3
	github.com/google/go-cmp v0.5.9
	github.com/google/go-containerregistry v0.4.1
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/slack-go/slack v0.8.2
	google.golang.org/api v0.125.0
	google.golang.org/protobuf v1.30.0
	gopkg.in/yaml.v2 v2.4.0
	gotest.tools/v3 v3.0.3 // indirect
	k8s.io/client-go v0.20.5
)

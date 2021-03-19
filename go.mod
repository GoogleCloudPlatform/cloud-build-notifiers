module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.16

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.79.0
	cloud.google.com/go/bigquery v1.16.0
	cloud.google.com/go/storage v1.14.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.1
	github.com/google/cel-go v0.7.2
	github.com/google/go-cmp v0.5.5
	github.com/google/go-containerregistry v0.4.1
	github.com/slack-go/slack v0.8.2
	google.golang.org/api v0.42.0
	google.golang.org/genproto v0.0.0-20210319143718-93e7006c17a6
	google.golang.org/protobuf v1.26.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/client-go v0.20.5
)

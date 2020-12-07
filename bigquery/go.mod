module github.com/GoogleCloudPlatform/cloud-build-notifiers/bigquery

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.73.0
	cloud.google.com/go/bigquery v1.14.0
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000
	github.com/docker/cli v20.10.0-rc2+incompatible // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/go-containerregistry v0.2.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	google.golang.org/api v0.36.0
	google.golang.org/genproto v0.0.0-20201207150747-9ee31aac76e7
	google.golang.org/protobuf v1.25.0
	gotest.tools/v3 v3.0.3 // indirect
)

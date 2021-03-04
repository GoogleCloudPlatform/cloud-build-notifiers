module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.16

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.78.0
	cloud.google.com/go/bigquery v1.16.0
	cloud.google.com/go/storage v1.14.0
	github.com/antlr/antlr4 v0.0.0-20210304020514-0f8bddcbc14b // indirect
	github.com/docker/cli v20.10.5+incompatible // indirect
	github.com/docker/docker v20.10.5+incompatible // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/cel-go v0.7.2
	github.com/google/go-cmp v0.5.5
	github.com/google/go-containerregistry v0.4.1
	github.com/magefile/mage v1.11.0 // indirect
	github.com/sirupsen/logrus v1.8.0 // indirect
	github.com/slack-go/slack v0.8.1
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110 // indirect
	golang.org/x/oauth2 v0.0.0-20210220000619-9bb904979d93 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20210304152209-afaa3650a925 // indirect
	google.golang.org/api v0.40.0
	google.golang.org/genproto v0.0.0-20210303154014-9728d6b83eeb
	google.golang.org/grpc v1.36.0 // indirect
	google.golang.org/protobuf v1.25.0
	gopkg.in/yaml.v2 v2.4.0
	gotest.tools/v3 v3.0.3 // indirect
	k8s.io/client-go v0.20.4
)

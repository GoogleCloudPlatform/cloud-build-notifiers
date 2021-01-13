module github.com/GoogleCloudPlatform/cloud-build-notifiers/bigquery

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.75.0
	cloud.google.com/go/bigquery v1.14.0
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-20201207173907-e18059bc9a58
	github.com/antlr/antlr4 v0.0.0-20210105212045-464bcbc32de2 // indirect
	github.com/docker/cli v20.10.2+incompatible // indirect
	github.com/docker/docker v20.10.2+incompatible // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/go-containerregistry v0.3.0
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	golang.org/x/oauth2 v0.0.0-20210113160501-8b1d76fa0423 // indirect
	golang.org/x/sys v0.0.0-20210113181707-4bcb84eeeb78 // indirect
	golang.org/x/text v0.3.5 // indirect
	golang.org/x/tools v0.0.0-20210113180300-f96436850f18 // indirect
	google.golang.org/api v0.36.0
	google.golang.org/genproto v0.0.0-20210113155445-facbc42f5e06
	google.golang.org/grpc v1.34.1 // indirect
	google.golang.org/protobuf v1.25.0
	gotest.tools/v3 v3.0.3 // indirect
)

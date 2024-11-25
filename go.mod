module github.com/GoogleCloudPlatform/cloud-build-notifiers

go 1.19

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ./lib/notifiers

require (
	cloud.google.com/go v0.81.0
	cloud.google.com/go/bigquery v1.16.0
	cloud.google.com/go/storage v1.14.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/google/cel-go v0.7.3
	github.com/google/go-cmp v0.5.5
	github.com/google/go-containerregistry v0.4.1
	github.com/slack-go/slack v0.8.2
	google.golang.org/api v0.43.0
	google.golang.org/genproto v0.0.0-20210825212027-de86158e7fda
	google.golang.org/protobuf v1.28.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/client-go v0.20.5
)

require (
	github.com/antlr/antlr4 v0.0.0-20210404160547-4dfacf63e228 // indirect
	github.com/docker/cli v20.10.5+incompatible // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.5+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/jstemmer/go-junit-report v0.9.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	golang.org/x/oauth2 v0.0.0-20210402161424-2e8d93401602 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/tools v0.1.5 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.40.0 // indirect
	gotest.tools/v3 v3.0.3 // indirect
)

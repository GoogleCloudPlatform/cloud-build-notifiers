module github.com/GoogleCloudPlatform/cloud-build-notifiers/smtp-template

go 1.18

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../

require (
	github.com/GoogleCloudPlatform/cloud-build-notifiers v0.0.0-20220422205219-926d83e64f1e
	github.com/golang/glog v1.0.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.7
	google.golang.org/genproto v0.0.0-20220422154200-b37d22cd5731
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cloud.google.com/go v0.81.0 // indirect
	cloud.google.com/go/storage v1.14.0 // indirect
	github.com/antlr/antlr4 v0.0.0-20210404160547-4dfacf63e228 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/cel-go v0.7.3 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/jstemmer/go-junit-report v0.9.1 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	golang.org/x/oauth2 v0.0.0-20210402161424-2e8d93401602 // indirect
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/tools v0.1.5 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.43.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/grpc v1.45.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	k8s.io/client-go v0.20.5 // indirect
)

module github.com/GoogleCloudPlatform/cloud-build-notifiers/bigquery

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.76.0
	cloud.google.com/go/bigquery v1.15.0
	cloud.google.com/go/storage v1.13.0 // indirect
	github.com/Azure/azure-sdk-for-go v42.3.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.10.2 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/antlr/antlr4 v0.0.0-20210203043838-a60c32d36933 // indirect
	github.com/aws/aws-sdk-go v1.31.6 // indirect
	github.com/docker/cli v20.10.3+incompatible // indirect
	github.com/docker/docker v20.10.3+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/cel-go v0.7.1 // indirect
	github.com/google/go-containerregistry v0.4.0
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	github.com/vdemeester/k8s-pkg-credentialprovider v1.18.1-0.20201019120933-f1d16962a4db // indirect
	go.opencensus.io v0.22.6 // indirect
	golang.org/x/oauth2 v0.0.0-20210201163806-010130855d6c // indirect
	golang.org/x/text v0.3.5 // indirect
	gonum.org/v1/netlib v0.0.0-20190331212654-76723241ea4e // indirect
	google.golang.org/api v0.39.0
	google.golang.org/genproto v0.0.0-20210204154452-deb828366460
	google.golang.org/protobuf v1.25.0
	gotest.tools/v3 v3.0.3 // indirect
	sigs.k8s.io/structured-merge-diff v0.0.0-20190525122527-15d366b2352e // indirect
)

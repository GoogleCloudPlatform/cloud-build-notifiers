module github.com/GoogleCloudPlatform/cloud-build-notifiers/http

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.76.0 // indirect
	cloud.google.com/go/storage v1.13.0 // indirect
	github.com/antlr/antlr4 v0.0.0-20210203043838-a60c32d36933 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/google/cel-go v0.7.1 // indirect
	go.opencensus.io v0.22.6 // indirect
	golang.org/x/oauth2 v0.0.0-20210201163806-010130855d6c // indirect
	golang.org/x/text v0.3.5 // indirect
	google.golang.org/api v0.39.0 // indirect
	google.golang.org/genproto v0.0.0-20210204154452-deb828366460
	google.golang.org/protobuf v1.25.0
)

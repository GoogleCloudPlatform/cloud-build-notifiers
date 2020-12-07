module github.com/GoogleCloudPlatform/cloud-build-notifiers/http

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	google.golang.org/genproto v0.0.0-20201207150747-9ee31aac76e7
	google.golang.org/protobuf v1.25.0
)

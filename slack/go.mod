module github.com/GoogleCloudPlatform/cloud-build-notifiers/slack

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp v0.5.4
	github.com/pkg/errors v0.9.1 // indirect
	github.com/slack-go/slack v0.7.2
	google.golang.org/genproto v0.0.0-20201207150747-9ee31aac76e7
)

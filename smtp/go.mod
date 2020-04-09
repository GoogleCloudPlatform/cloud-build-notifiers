module github.com/GoogleCloudPlatform/cloud-build-notifiers/smtp

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.3
	github.com/google/go-cmp v0.4.0
	google.golang.org/genproto v0.0.0-20200205142000-a86caf926a67
	gopkg.in/yaml.v2 v2.2.8
)

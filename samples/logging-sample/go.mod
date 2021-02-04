module github.com/GoogleCloudPlatform/cloud-build-notifiers/logging-sample

go 1.15

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000 // indirect
	google.golang.org/genproto v0.0.0-20210126160654-44e461bb6506 // indirect
)

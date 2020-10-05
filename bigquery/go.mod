module github.com/GoogleCloudPlatform/cloud-build-notifiers/bigquery

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.63.0
	cloud.google.com/go/bigquery v1.10.0
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-00010101000000-000000000000
	github.com/antlr/antlr4 v0.0.0-20200801005519-2ba38605b949 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.2
	github.com/google/cel-go v0.5.1 // indirect
	github.com/google/go-containerregistry v0.1.1
	golang.org/x/sys v0.0.0-20200812155832-6a926be9bd1d // indirect
	golang.org/x/tools v0.0.0-20200812195022-5ae4c3c160a0 // indirect
	google.golang.org/api v0.30.0
	google.golang.org/genproto v0.0.0-20200812160120-2e714abc8b50
	google.golang.org/protobuf v1.25.0
)

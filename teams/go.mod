module github.com/GoogleCloudPlatform/cloud-build-notifiers/teams

go 1.14

replace github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers => ../lib/notifiers

require (
	cloud.google.com/go v0.62.0 // indirect
	github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers v0.0.0-20200731210753-3c7e9032cb03
	github.com/antlr/antlr4 v0.0.0-20200801005519-2ba38605b949 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.2
	github.com/google/cel-go v0.5.1 // indirect
	golang.org/x/exp v0.0.0-20200513190911-00229845015e // indirect
	golang.org/x/sys v0.0.0-20200803150936-fd5f0c170ac3 // indirect
	golang.org/x/tools v0.0.0-20200731060945-b5fad4ed8dd6 // indirect
	google.golang.org/genproto v0.0.0-20200731012542-8145dea6a485
	google.golang.org/grpc v1.31.0 // indirect
	google.golang.org/protobuf v1.25.0
	gopkg.in/yaml.v2 v2.3.0 // indirect
	honnef.co/go/tools v0.0.1-2020.1.5 // indirect
)

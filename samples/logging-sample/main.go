package main

import ("context"
        "flag"
        "github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
        log "github.com/golang/glog"
        cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
        "fmt"
        "github.com/golang/protobuf/proto"
)

func init(){
    // To parse the flags for golang/glog
    flag.Parse()
}

func main() {
    if err := notifiers.Main(new(logger)); err != nil {
        log.Fatalf("fatal error: %v", err)
    }
}

type logger struct {
    filter notifiers.EventFilter
}

func (h *logger) SetUp(_ context.Context, cfg *notifiers.Config, _ notifiers.SecretGetter, _ notifiers.BindingResolver) error {
    prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
     if err != nil {
        return fmt.Errorf("failed to create CELPredicate: %w", err)
     }
    h.filter = prd
    return nil
}

func (h *logger) SendNotification(ctx context.Context, build *cbpb.Build) error {
    // Include custom functionality here.
    // This example logs the build.
    if h.filter.Apply(ctx, build) {
        log.V(1).Infof("printing build\n%s", proto.MarshalTextString(build))
    } else {
        log.V(1).Infof("build (%q, %q) did NOT match CEL filter", build.ProjectId, build.Id)
    }

    log.Flush()
    return nil
}

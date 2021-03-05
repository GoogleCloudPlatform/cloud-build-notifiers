// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// [START cloudbuild_logging_sample_main]
// [START cloudbuild_logging_sample_imports]
package main

import (
        "context"
        "fmt"

        cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
        log "github.com/golang/glog"
        "github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
        "github.com/golang/protobuf/proto"
)
// [END cloudbuild_logging_sample_imports]

// [START cloudbuild_logging_sample_main_func]
func main() {
    if err := notifiers.Main(new(logger)); err != nil {
        log.Fatalf("fatal error: %v", err)
    }
}
// [END cloudbuild_logging_sample_main_func]

// [START cloudbuild_logging_sample_struct]
type logger struct {
    filter notifiers.EventFilter
}
// [END cloudbuild_logging_sample_struct]

// [START cloudbuild_logging_sample_setup_notify]
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

    return nil
}
// [END cloudbuild_logging_sample_setup_notify]
// [END cloudbuild_logging_sample_main]

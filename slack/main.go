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

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	webhookURLSecretName = "webhookUrl"
)

func main() {
	if err := notifiers.Main(new(slackNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type slackNotifier struct {
	filter notifiers.EventFilter

	webhookURL string
}

func (s *slackNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	s.filter = prd

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, webhookURLSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, webhookURLSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %w", err)
	}
	s.webhookURL = wu

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !s.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)
	msg, err := s.writeMessage(build)
	if err != nil {
		return fmt.Errorf("failed to write Slack message: %w", err)
	}

	return slack.PostWebhook(s.webhookURL, msg)
}

func (s *slackNotifier) writeMessage(build *cbpb.Build) (*slack.WebhookMessage, error) {

	commitURL := fmt.Sprintf("https://github.com/CartoDB/%s/commit/%s", build.Substitutions["REPO_NAME"], build.Substitutions["COMMIT_SHA"])
	logURL := build.LogUrl

	var clr string
	var buildStatus string

	switch build.Status {
	case cbpb.Build_SUCCESS:
		buildStatus = fmt.Sprintf("%s :heavy_check_mark:", build.Status)
		clr = "good"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		buildStatus = fmt.Sprintf("%s :facepalm-jenkins:", build.Status)
		clr = "danger"
	default:
		buildStatus = build.Status.String()
		clr = "warning"
	}

	var txt strings.Builder
	txt.WriteString(fmt.Sprintf(
		"Cloud Build (%s, %s):\n*%s*\n",
		build.ProjectId,
		build.Id,
		buildStatus,
	))

	format := "2006-01-02T15:04:05Z07:00"
	finishTime := time.Parse(format, string(build.FinishTime))
	startTime := time.Parse(format, string(build.StartTime))
	buildDuration := finishTime.Sub(startTime)
	
	txt.WriteString(fmt.Sprintf("- duration: %s\n", buildDuration))
	txt.WriteString(fmt.Sprintf("- repository: %s\n", build.Substitutions["REPO_NAME"]))
	txt.WriteString(fmt.Sprintf("- branch: %s\n", build.Substitutions["BRANCH_NAME"]))
	txt.WriteString(fmt.Sprintf("- commit: %s\n", build.Substitutions["SHORT_SHA"]))

	atch := slack.Attachment{
		Text:  txt.String(),
		Color: clr,
		Actions: []slack.AttachmentAction{
            slack.AttachmentAction{
                Name:  "Build logs",
                Text:  "Build logs",
                Type:  "button",
                URL: logURL,
            },
            slack.AttachmentAction{
                Name:  "Commit",
                Text:  "Github commit",
                Type:  "button",
                URL: commitURL,
            },
        },
	}

	return &slack.WebhookMessage{Attachments: []slack.Attachment{atch}}, nil
}

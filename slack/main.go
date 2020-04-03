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
	"time"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/nlopes/slack"
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

func (s *slackNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %v", err)
	}
	s.filter = prd

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, webhookURLSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %v", cfg.Spec.Notification.Delivery, webhookURLSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %v", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %v", err)
	}
	s.webhookURL = wu

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, event *notifiers.CloudBuildEvent) error {
	if !s.filter.Apply(ctx, event) {
		return nil
	}

	log.Infof("sending Slack webhook for Build %q (status: %q; created at: %s)", event.ID, event.Status, event.CreateTime.Format(time.RFC3339))
	msg, err := s.writeMessage(event)
	if err != nil {
		return fmt.Errorf("failed to write Slack message: %v", err)
	}

	return slack.PostWebhook(s.webhookURL, msg)
}

func (s *slackNotifier) writeMessage(event *notifiers.CloudBuildEvent) (*slack.WebhookMessage, error) {
	txt := fmt.Sprintf(
		"Cloud Build (%s, %s): %s",
		event.ProjectID,
		event.ID,
		event.Status,
	)

	var clr string
	switch event.Status {
	case "SUCCESS":
		clr = "good"
	case "FAILURE", "INTERNAL_ERROR", "TIMEOUT":
		clr = "danger"
	default:
		clr = "warning"
	}

	logURL, err := notifiers.AddUTMParams(event.LogURL, notifiers.ChatMedium)
	if err != nil {
		return nil, fmt.Errorf("failed to add UTM params: %v", err)
	}

	atch := slack.Attachment{
		Text:  txt,
		Color: clr,
		Actions: []slack.AttachmentAction{slack.AttachmentAction{
			Text: "View Logs", Type: "button", URL: logURL,
		}},
	}

	return &slack.WebhookMessage{Attachments: []slack.Attachment{atch}}, nil
}

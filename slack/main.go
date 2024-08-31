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
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/slack-go/slack"
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
	filter     notifiers.EventFilter
	tmpl       *template.Template
	webhookURL string
	br         notifiers.BindingResolver
	tmplView   *notifiers.TemplateView
}

func (s *slackNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, blockKitTemplate string, sg notifiers.SecretGetter, br notifiers.BindingResolver) error {
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
	tmpl, err := template.New("blockkit_template").Funcs(template.FuncMap{
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"slice": func(s string, start, end int) string {
			if start < 0 {
				start = 0
			}
			if end > len(s) {
				end = len(s)
			}
			if start > end {
				return ""
			}
			return s[start:end]
		},
	}).Parse(blockKitTemplate)

	s.tmpl = tmpl
	s.br = br

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {

	if !s.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)

	log.Infof("Log build: %+v", build)
	log.Infof("Log context: %+v", ctx)

	bindings, err := s.br.Resolve(ctx, nil, build)
	if err != nil {
		return fmt.Errorf("failed to resolve bindings: %w", err)
	}

	s.tmplView = &notifiers.TemplateView{
		Build:  &notifiers.BuildView{Build: build},
		Params: bindings,
	}

	msg, err := s.writeMessage()

	if err != nil {
		return fmt.Errorf("failed to write Slack message: %w", err)
	}

	return slack.PostWebhook(s.webhookURL, msg)
}

func (s *slackNotifier) writeMessage() (*slack.WebhookMessage, error) {
	build := s.tmplView.Build
	_, err := notifiers.AddUTMParams(build.LogUrl, notifiers.ChatMedium)

	if err != nil {
		return nil, fmt.Errorf("failed to add UTM params: %w", err)
	}

	var clr string
	var colourCode string
	var buildDuration string

	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "🟢"
		colourCode = "#0DBE0C"
		buildDuration = formatDuration(int(build.FinishTime.Seconds) - int(build.StartTime.Seconds))
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT, cbpb.Build_EXPIRED, cbpb.Build_CANCELLED:
		clr = "🔴"
		colourCode = "#AE1413"
		buildDuration = formatDuration(int(time.Now().Unix()) - int(build.StartTime.Seconds))
	default:
		clr = "🟠"
		colourCode = "#DE7A00"
	}

	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, s.tmplView); err != nil {
		return nil, err
	}
	var blocks slack.Blocks

	err = blocks.UnmarshalJSON(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal templating JSON: %w", err)
	}

	log.Infof("Block in writeMessage() %+v", blocks)
	log.Infof("clr %q", clr)

	// log.Infof("Ts field: %+v", time.Now().UnixMilli())
	log.Infof("Ts field: %+v", build.GetCreateTime().AsTime())
	log.Infof("timing field: %+v", build.GetTiming())

	var messageParts []string

	// Helper function to append non-empty values
	wrapWith := func(part string, startWrapChar string, endWrapChar string) {
		if part != "" && part != "<nil>" {
			part = strings.ReplaceAll(part, "\n", "_")
			messageParts = append(messageParts, fmt.Sprintf("%s%s%s", startWrapChar, part, endWrapChar))
		}
	}

	wrapWith(clr, "", "")
	wrapWith(build.Status.String(), "*", "*")
	if build.Substitutions["REPO_NAME"] != "" && build.Substitutions["BRANCH_NAME"] != "" {
		wrapWith(build.Substitutions["REPO_NAME"]+"/"+build.Substitutions["BRANCH_NAME"], "– `", "`")
	} else if build.Substitutions["TRIGGER_NAME"] != "" {
		wrapWith(build.Substitutions["TRIGGER_NAME"], "– `", "`")
	} else {
		wrapWith("Trigger manually", "– ", "")
		wrapWith(build.Substitutions["REF_NAME"], "`", "`")
	}

	wrapWith(build.Substitutions["_COMMIT_MESSAGE"], "– _\"", "\"_")
	wrapWith(buildDuration, "– _", "_")
	wrapWith(build.GetFailureInfo().String(), "\n> *Error*: _\"", "\"_")

	// Create message text without unnecessary characters
	messageText := strings.Join(messageParts, " ")

	log.Infof("messageText: %+v", messageText)

	// attachments in Slack payload: https://api.slack.com/methods/chat.postMessage#arg_attachments
	return &slack.WebhookMessage{
		Text: messageText,
		Attachments: []slack.Attachment{
			{
				Color:  colourCode,
				Blocks: blocks,
			}},
	}, nil
}

func formatDuration(seconds int) string {
	minutes := seconds / 60
	remainingSeconds := seconds % 60
	return fmt.Sprintf("%dm%ds", minutes, remainingSeconds)
}

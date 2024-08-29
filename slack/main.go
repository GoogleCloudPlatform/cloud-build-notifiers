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

	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "🟢"
		colourCode = "#0DBE0C"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "🔴"
		colourCode = "#AE1413"
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

	commitMessage, exists := build.Substitutions["_COMMIT_MESSAGE"]
	if !exists {
		commitMessage = "" // Provide a default message if the key doesn't exist
	}

	repoName, exists := build.Substitutions["REPO_NAME"]
	if !exists {
		repoName = "" // Provide a default message if the key doesn't exist
	}

	branchName, exists := build.Substitutions["BRANCH_NAME"]
	if !exists {
		branchName = "" // Provide a default message if the key doesn't exist
	}

	// attachments in Slack payload: https://api.slack.com/methods/chat.postMessage#arg_attachments
	return &slack.WebhookMessage{
		Username: "GiaPhuThinh",
		Text:     fmt.Sprintf(`%s %s [%s] [%s] "%s"`, clr, build.Status.String(), repoName, branchName, commitMessage),
		Attachments: []slack.Attachment{
			{
				Color:  colourCode,
				Blocks: blocks,
			}},
	}, nil
}

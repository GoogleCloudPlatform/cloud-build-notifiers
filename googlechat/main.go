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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	chat "google.golang.org/api/chat/v1"

	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	webhookURLSecretName = "webhookUrl"
)

func main() {
	if err := notifiers.Main(new(googlechatNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type googlechatNotifier struct {
	filter notifiers.EventFilter

	webhookURL string
}

func (g *googlechatNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, sg notifiers.SecretGetter, _ notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	g.filter = prd

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
	g.webhookURL = wu

	return nil
}

func (g *googlechatNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !g.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Google Chat webhook for Build %q (status: %q)", build.Id, build.Status)
	msg, err := g.writeMessage(build)
	if err != nil {
		return fmt.Errorf("failed to write Google Chat message: %w", err)
	}

	payload := new(bytes.Buffer)
	err = json.NewEncoder(payload).Encode(msg)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.webhookURL, payload)
	if err != nil {
		return fmt.Errorf("failed to create a new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GCB-Notifier/0.1 (http)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warningf("got a non-OK response status %q (%d) from %q", resp.Status, resp.StatusCode, g.webhookURL)
	}

	log.V(2).Infoln("send HTTP request successfully")
	return nil
}

func (g *googlechatNotifier) writeMessage(build *cbpb.Build) (*chat.Message, error) {

	var icon string

	switch build.Status {
	case cbpb.Build_SUCCESS:
		icon = "https://www.gstatic.com/images/icons/material/system/2x/check_circle_googgreen_48dp.png"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR:
		icon = "https://www.gstatic.com/images/icons/material/system/2x/error_red_48dp.png"
	case cbpb.Build_TIMEOUT:
		icon = "https://www.gstatic.com/images/icons/material/system/2x/hourglass_empty_black_48dp.png"
	default:
		icon = "https://www.gstatic.com/images/icons/material/system/2x/question_mark_black_48dp.png"
	}

	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.ChatMedium)
	if err != nil {
		return nil, fmt.Errorf("failed to add UTM params: %w", err)
	}

	// Basic card setup
	duration := build.GetFinishTime().AsTime().Sub(build.GetStartTime().AsTime())
	duration_min, duration_sec := int(duration.Minutes()), int(duration.Seconds())-int(duration.Minutes())*60
	duration_fmt := fmt.Sprintf("%d min %d sec", duration_min, duration_sec)

	card := &chat.Card{
		Header: &chat.CardHeader{
			Title:    fmt.Sprintf("Build %s Status: %s", build.Id[:8], build.Status),
			Subtitle: build.ProjectId,
			ImageUrl: icon,
		},
		Sections: []*chat.Section{
			{
				Widgets: []*chat.WidgetMarkup{
					{
						KeyValue: &chat.KeyValue{
							TopLabel: "Duration",
							Content:  duration_fmt,
						},
					},
				},
			},
		},
	}

	// Optional section: display trigger information
	if build.BuildTriggerId != "" {

		log.Infof("Detected a build trigger id: %s", build.BuildTriggerId)

		/*
			//TODO(glasnt): Get trigger information for Uri links.
			//  The repo name in `build` does not include the owner information
			//  You need to inspect the trigger object to get the full repo name and/or the git URI.

			ctx := context.Background()
			cbapi, _ := cloudbuild.NewClient(ctx)
			trigger_info := cbapi.GetBuildTrigger(ctx, &cbpb.GetBuildTriggerRequest{ProjectId: build.ProjectId, TriggerId: build.BuildTriggerId,})
			log.Infof("Trigger Repo URI: %s", trigger_info.??)
		*/

		repo_name := build.Substitutions["REPO_NAME"]
		trigger_name := build.Substitutions["TRIGGER_NAME"]
		commit := build.Substitutions["SHORT_SHA"]

		// Branch, Tag, or None.
		branch_tag_label := "Branch"
		branch_tag_value := build.Substitutions["BRANCH_NAME"]

		if branch_tag_value == "" {
			branch_tag_label = "Tag"
			branch_tag_value = build.Substitutions["TAG_NAME"]

			if branch_tag_value == "" {
				branch_tag_label = "Branch/Tag"
				branch_tag_value = "[no branch or tag]"
			}
		}

		card.Header.Subtitle = fmt.Sprintf("%s on %s", trigger_name, build.ProjectId)

		build_info := &chat.Section{
			Header: "Trigger information",
			Widgets: []*chat.WidgetMarkup{
				{
					KeyValue: &chat.KeyValue{
						TopLabel: "Trigger",
						Content:  trigger_name,
					},
				},
				{
					KeyValue: &chat.KeyValue{
						TopLabel: "Repo",
						Content:  repo_name,
					},
				},
				{
					KeyValue: &chat.KeyValue{
						TopLabel: branch_tag_label,
						Content:  branch_tag_value,
					},
				},
				{
					KeyValue: &chat.KeyValue{
						TopLabel: "Commit",
						Content:  commit,
					},
				},
			},
		}

		card.Sections = append(card.Sections, build_info)
	}

	// Optional section: display information about errors
	if build.FailureInfo != nil {
		failure_info := &chat.Section{
			Header: "Error information",
			Widgets: []*chat.WidgetMarkup{
				{
					TextParagraph: &chat.TextParagraph{
						Text: build.FailureInfo.GetDetail(),
					},
				},
			},
		}
		card.Sections = append(card.Sections, failure_info)
	}

	// Append action button
	action_section := &chat.Section{
		Widgets: []*chat.WidgetMarkup{
			{
				Buttons: []*chat.Button{
					{
						TextButton: &chat.TextButton{
							Text: "open logs",
							OnClick: &chat.OnClick{
								OpenLink: &chat.OpenLink{
									Url: logURL,
								},
							},
						},
					},
				},
			},
		},
	}

	card.Sections = append(card.Sections, action_section)

	msg := chat.Message{Cards: []*chat.Card{card}}
	return &msg, nil
}

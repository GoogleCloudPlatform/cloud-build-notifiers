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

	"io/ioutil"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/slack-go/slack"
	cbpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

const (
	webhookURLSecretName          = "webhookUrl"
	notificationChannelConfigName = "notificationChannel"
	apiTokenSecretName            = "apiToken"
	storagePathPrefixf            = "messages/%s"
)

func main() {
	if err := notifiers.Main(new(slackNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type slackNotifier struct {
	filter notifiers.EventFilter

	webhookURL          string
	notificationChannel string
	apiToken            string

	storageBucket string
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

	channelRef, ok := cfg.Spec.Notification.Delivery[notificationChannelConfigName]
	if ok {
		s.notificationChannel, _ = channelRef.(string)
	}

	apiRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, apiTokenSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, apiTokenSecretName, err)
	}
	apiResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, apiRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", apiRef, err)
	}
	apiToken, err := sg.GetSecret(ctx, apiResource)
	if err != nil {
		return fmt.Errorf("failed to get api secret: %w", err)
	}
	s.apiToken = apiToken

	cfgPath := os.Getenv("CONFIG_PATH")
	if trm := strings.TrimPrefix(cfgPath, "gs://"); trm != cfgPath {
		cfgPath = trm
		split := strings.SplitN(cfgPath, "/", 2)
		s.storageBucket = split[0]
	}

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) (err error) {
	if !s.filter.Apply(ctx, build) {
		return nil
	}

	slackClient := slack.New(s.apiToken)

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)
	attachmentMsgOpt := s.buildAttachmentMessageOption(build)
	timestamp := s.getTimestamp(build.Id)
	if timestamp == "" {
		_, timestamp, err = slackClient.PostMessage(s.notificationChannel, *attachmentMsgOpt)
		s.setTimestamp(build.Id, timestamp)
	} else {
		_, _, _, err = slackClient.UpdateMessage(s.notificationChannel, timestamp, *attachmentMsgOpt)
		s.deleteTimestamp(build.Id)
	}
	return
}

func (s *slackNotifier) getStoragePath(buildId string) string {
	return fmt.Sprintf(storagePathPrefixf, buildId)
}

func (s *slackNotifier) deleteTimestamp(buildId string) {
	sc, err := storage.NewClient(context.Background())
	if err != nil {
		return
	}
	defer sc.Close()

	if err := sc.Bucket(s.storageBucket).Object(s.getStoragePath(buildId)).Delete(context.Background()); err != nil {
		log.Infof("Error deleting the object: %q", err.Error())
		return
	}
	return
}

func (s *slackNotifier) setTimestamp(buildId string, timestamp string) {
	sc, err := storage.NewClient(context.Background())
	if err != nil {
		log.Infof("Unable to create storage client: %q", err.Error())
		return
	}
	defer sc.Close()

	writer := sc.Bucket(s.storageBucket).Object(s.getStoragePath(buildId)).NewWriter(context.Background())
	defer func() {
		err := writer.Close()
		if err != nil {
			log.Infof("Error closing the writer: %q", err.Error())
		}
	}()
	if _, err := fmt.Fprint(writer, timestamp); err != nil {
		return
	}
}

func (s *slackNotifier) getTimestamp(buildId string) (timestamp string) {
	sc, err := storage.NewClient(context.Background())
	if err != nil {
		return
	}
	defer sc.Close()

	path := s.getStoragePath(buildId)
	//r, err := sc.NewReader(ctx, bucket, path)
	reader, err := sc.Bucket(s.storageBucket).Object(path).NewReader(context.Background())
	if err != nil {
		return
	}
	defer reader.Close()

	var b []byte
	// will move to "io" in golang 1.16
	if b, err = ioutil.ReadAll(reader); err != nil {
		return
	}
	timestamp = string(b)

	return
}

func (s *slackNotifier) buildAttachmentMessageOption(build *cbpb.Build) *slack.MsgOption {
	repoName, ok := build.Substitutions["REPO_NAME"]
	if !ok {
		repoName = "UNKNOWN_REPO"
	}
	branchName, ok := build.Substitutions["BRANCH_NAME"]
	if !ok {
		branchName = "UNKNOWN_BRANCH"
	}
	commitSha, ok := build.Substitutions["SHORT_SHA"]
	if !ok {
		commitSha = "UNKNOWN_COMMIT_SHA"
	}
	commitMsg, ok := build.Substitutions["_COMMIT_MESSAGE"]
	if !ok {
		commitMsg = "UNKNOWN_COMMIT_MESSAGE"
	}
	commitURL, ok := build.Substitutions["_COMMIT_URL"]
	if !ok {
		commitURL = "UNKNOWN_COMMIT_URL"
	}
	commitAuthor, ok := build.Substitutions["_COMMIT_AUTHOR"]
	if !ok {
		commitAuthor = "UNKNOWN_COMMIT_AUTHOR"
	}

	logURL, err := notifiers.AddUTMParams(build.LogUrl, notifiers.ChatMedium)
	if err != nil {
		logURL = build.LogUrl
	}

	txt := fmt.Sprintf(
		"%s: :%s: %s (%s) <%s|View Build>\n*Branch*: %s *Author*: %s \n<%s|Commit> *%s*: %s",
		build.Status,
		repoName,
		repoName,
		build.ProjectId,
		logURL,
		branchName,
		commitAuthor,
		commitURL,
		commitSha,
		commitMsg,
	)

	var clr string
	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "good"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "danger"
	default:
		clr = "warning"
	}

	attachment := slack.Attachment{
		Text:  txt,
		Color: clr,
	}

	attachmentMsgOption := slack.MsgOptionAttachments(attachment)
	return &attachmentMsgOption
}

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
	"encoding/json"
	"fmt"

	"io"
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
	repoNameSub                   = "REPO_NAME"
	branchNameSub                 = "BRANCH_NAME"
	commitShortShaSub             = "SHORT_SHA"
	commitShaSub                  = "COMMIT_SHA"
	commitMsgSub                  = "_COMMIT_MESSAGE"
	commitURLSub                  = "_COMMIT_URL"
	commitAuthorSub               = "_COMMIT_AUTHOR"
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
	slackClient         *slack.Client

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

	s.slackClient = slack.New(apiToken)

	cfgPath := os.Getenv("CONFIG_PATH")
	if trm := strings.TrimPrefix(cfgPath, "gs://"); trm != cfgPath {
		cfgPath = trm
		split := strings.SplitN(cfgPath, "/", 2)
		s.storageBucket = split[0]
	}

	return nil
}

// Used to store build information in google cloud storage
type storedBuild struct {
	Timestamp string                 `json:"timestamp"`
	Build     map[string]*cbpb.Build `json:"build"`
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {
	if !s.filter.Apply(ctx, build) {
		return nil
	}

	// Create the google cloud storage client
	sc, err := storage.NewClient(context.Background())
	if err != nil {
		log.Infof("Unable to create storage client : %q", err.Error())
		return err
	}
	defer sc.Close()

	// Get the commit sha for the storage file name for deploybot
	commitSha, ok := build.Substitutions[commitShaSub]
	if !ok {
		return fmt.Errorf("Unknown %s", commitShaSub)
	}

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)

	// Get the stored builds from google cloud storage if it exists
	sb := s.getStoredBuild(sc, commitSha)
	// Add the latest build to the stored build info
	sb.Build[build.Id] = build
	// Create the message to send to slack
	attachmentMsgOpt := buildAttachmentMessageOption(sb)
	// If no initial message has been sent to slack yet
	if sb.Timestamp == "" {
		// Send the initial message
		_, timestamp, err := s.slackClient.PostMessage(s.notificationChannel, *attachmentMsgOpt)
		if err != nil {
			log.Infof("Unable to post initial build message to slack : %q", err.Error())
		}
		// Set the slack timestamp (the unique identifier slack uses for channel messages)
		sb.Timestamp = timestamp
		// Store the initial build information in google cloud
		return s.updateCloudStoreFile(sc, commitSha, sb)
	}

	// Update the slack message
	_, _, _, err = s.slackClient.UpdateMessage(s.notificationChannel, sb.Timestamp, *attachmentMsgOpt)
	if err != nil {
		log.Infof("Unable to update slack message : %q", err.Error())
	}

	// Update the stored build information
	err = s.updateCloudStoreFile(sc, commitSha, sb)
	if err != nil {
		return err
	}

	return err
}

// getStoredBuild fetches the build info for this commit hash from google cloud storage
// and adds the latest build to the response
func (s *slackNotifier) getStoredBuild(sc *storage.Client, commitSha string) storedBuild {
	path := getStoragePath(commitSha)
	sb := storedBuild{Build: map[string]*cbpb.Build{}}
	// Open a reader for the file in the google store, if it exists
	reader, err := sc.Bucket(s.storageBucket).Object(path).NewReader(context.Background())
	if err != nil {
		// If it doesn't exist yet, just log that info, we can create it later
		log.Infof("Could not read stored file (may not exist yet) (%s) in bucket (%s) : %q", path, s.storageBucket, err.Error())
		return sb
	}
	defer reader.Close()
	// Read and unmarshal the file
	sb, err = readFromBuildStorageFile(reader, sb)
	if err != nil {
		log.Infof("Unable to read/unmarshal file (%s) in bucket (%s) : %q", s.storageBucket, path, err.Error())
	}
	return sb
}

// updateCloudStore updates google cloud storage with the latest build info under the commit sha filename
func (s *slackNotifier) updateCloudStoreFile(sc *storage.Client, commitSha string, sb storedBuild) error {
	writer := sc.Bucket(s.storageBucket).Object(getStoragePath(commitSha)).NewWriter(context.Background())
	defer writer.Close()

	return writeBuildToStorageFile(writer, sb)
}

func getStoragePath(commitSha string) string {
	return fmt.Sprintf(storagePathPrefixf, commitSha)
}

// readFromBuildStorageFile reads the json from the build storage file and adds the latest build
func readFromBuildStorageFile(reader io.Reader, sb storedBuild) (storedBuild, error) {
	var err error
	var b []byte
	if b, err = io.ReadAll(reader); err != nil {
		log.Infof("Unable to read from file : %q", err.Error())
		return sb, err
	}
	if err := json.Unmarshal(b, &sb); err != nil {
		log.Infof("Unable to unmarshal json : %q", err.Error())
		return sb, err
	}

	return sb, nil
}

// writeBuildToStorageFile writes the json to the build storage file
func writeBuildToStorageFile(writer io.Writer, sb storedBuild) error {
	// Marshal the timestamp/build info
	b, err := json.Marshal(sb)
	if err != nil {
		log.Infof("Unable to marshal : %q", err.Error())
		return err
	}

	// Write the timestamp/build info file to the storage file
	if _, err := fmt.Fprint(writer, string(b)); err != nil {
		log.Infof("Unable to write to cloud storage : %q", err.Error())
	}

	return err
}

// statusInProgress checks if the given status is an "in progress" status
func statusInProgress(status cbpb.Build_Status) bool {
	inProgressStatuses := []cbpb.Build_Status{
		cbpb.Build_WORKING,
		cbpb.Build_PENDING,
		cbpb.Build_QUEUED,
	}
	for _, ps := range inProgressStatuses {
		if status == ps {
			return true
		}
	}
	return false
}

func buildAttachmentMessageOption(sb storedBuild) *slack.MsgOption {
	// Default values for the build info fields
	buildInfo := map[string]string{
		repoNameSub:       "UNKNOWN_REPO",
		branchNameSub:     "UNKNOWN_BRANCH",
		commitShortShaSub: "UNKNOWN_COMMIT_SHA",
		commitMsgSub:      "UNKNOWN_COMMIT_MESSAGE",
		commitURLSub:      "UNKNOWN_COMMIT_URL",
		commitAuthorSub:   "UNKNOWN_COMMIT_AUTHOR",
	}
	buildLogUrl := ""
	buildStatus := cbpb.Build_STATUS_UNKNOWN
	buildProjectId := ""
	// Look at all the builds
	// (named this loop as buildLoop so we can break out of it in the switch case)
buildLoop:
	for _, build := range sb.Build {
		// Check all the build substition info fields
		for key := range buildInfo {
			// If there is a value for that build key in the build info
			if val, ok := build.Substitutions[key]; ok {
				// Update value for that key
				buildInfo[key] = val
			}
		}
		// Capture the other fields we need for our message
		buildLogUrl = build.LogUrl
		buildStatus = build.Status
		buildProjectId = build.ProjectId
		// If one of the builds failed, this should be the primary message
		switch buildStatus {
		case
			cbpb.Build_FAILURE,
			cbpb.Build_TIMEOUT,
			cbpb.Build_INTERNAL_ERROR:
			// Stop looking through other builds
			break buildLoop
		}
	}

	logURL, err := notifiers.AddUTMParams(buildLogUrl, notifiers.ChatMedium)
	if err != nil {
		logURL = buildLogUrl
	}

	txt := fmt.Sprintf(
		"%s: :%s: %s (%s) <%s|View Build>\n*Branch*: %s *Author*: %s \n<%s|Commit> *%s*: %s",
		buildStatus,
		buildInfo[repoNameSub],
		buildInfo[repoNameSub],
		buildProjectId,
		logURL,
		buildInfo[branchNameSub],
		buildInfo[commitAuthorSub],
		buildInfo[commitURLSub],
		buildInfo[commitShortShaSub],
		buildInfo[commitMsgSub],
	)

	var clr string
	switch buildStatus {
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

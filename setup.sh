#!/bin/bash
# Copyright 2020 Google LLC
# Author: lru@google.com (Leo Rudberg)

set -u

HELP="
Cloud Build Notifiers setup script.

This bash script runs almost all of the setup required for configuring
and deploying a notifier on GCP. It is based on the guide here:
https://cloud.google.com/cloud-build/docs/configure-notifications.

Setting up any 'secret_name' must be done outside
this script. This script is assumed to be run in the root of your
cloud-build-notifiers clone/fork. Currently, this script only deploys
the 'latest' version of the notifier to Cloud Run. To check that your notifier
configuration YAML can work with that version, run the notifier locally with
the '--setup_check' flag that is mentioned in the repo root README.md. 

The currently supported notifier types (which correspond to the directories in
the repo) are:

* bigquery
* http
* slack
* smtp
* googlechat (alpha)
* githubissues (alpha)

Usage [in the cloud-build-notifiers repo root]:

./setup.sh <notifier-type> <local-config-path> [-s secret-name] [-t local-template-path]

Concrete example:

./setup.sh \
  smtp \
  ~/notifier-configs/my-smtp-config.yaml \
  -s my-smtp-password \
  -t ~/notifier-configs/my-smtp-template.yaml

For help/usage:

./setup.sh --help
"

main() {
  # Simple argument checks.
  if [ "$*" = "--help" ]; then
    echo "${HELP}"
    exit 0
  elif [ $# -lt 2 ]; then
    fail "${HELP}"
  fi

  SECRET_NAME=""
  SOURCE_TEMPLATE_PATH=""

  while [ $OPTIND -le "$#" ]
  do
    if getopts s:t: option
    then
        case $option
        in
            s) SECRET_NAME="$OPTARG";;
            t) SOURCE_TEMPLATE_PATH="$OPTARG";;
        esac
    else
        ((OPTIND++))
    fi
  done

  NOTIFIER_TYPE="$1"
  SOURCE_CONFIG_PATH="$2"

  # Check that the user is using a supported notifier type in the correct
  # directory.
  case "${NOTIFIER_TYPE}" in
  http | smtp | slack | bigquery | googlechat | githubissues) ;;
  *) fail "${HELP}" ;;
  esac

  if [ ! -d "${NOTIFIER_TYPE}" ]; then
    fail "expected to run from the root of the cloud-build-notifiers repo"
  fi

  if [ ! -r "${SOURCE_CONFIG_PATH}" ]; then
    fail "expected file at local source config path ${SOURCE_CONFIG_PATH} to be readable"
  fi

  if [ ! -z "${SOURCE_TEMPLATE_PATH}" ]; then
    if [ ! -r "${SOURCE_TEMPLATE_PATH}" ]; then
        fail "expected file at local source template path ${SOURCE_TEMPLATE_PATH} to be readable"
    fi
  fi

  # Project ID, assumed to NOT be org-scoped (only alphanumeric and dashes).
  PROJECT_ID=$(gcloud config get-value project) ||
    fail "could not get default project"
  if [ "${PROJECT_ID}" = "" ] || [ "${PROJECT_ID}" = "(unset)" ]; then
    fail "default project not set; run \"gcloud config set project <project_id>\"" \
      "or set the CLOUDSDK_CORE_PROJECT environment variable"
  fi
  if [ -z "${PROJECT_ID##*:*}" ]; then
    fail "org-scoped project IDs are not allowed by this script"
  fi
  echo "Fetching project number..."
  PROJECT_NUMBER=$(gcloud projects describe "${PROJECT_ID}" \
    --format="value(projectNumber)") ||
    fail "could not get project number"

  # Region, from Cloud Run configuration
  REGION=$(gcloud config get-value run/region)
  if [ "${REGION}" = "" ] || [ "${REGION}" = "(unset)" ]; then
    fail "default cloud run region not set; run \"gcloud config set run/region <region>\""
  fi

  REQUIRED_SERVICES=('cloudbuild.googleapis.com' 'run.googleapis.com' 'pubsub.googleapis.com')
  SOURCE_CONFIG_BASENAME=$(basename "${SOURCE_CONFIG_PATH}")
  DESTINATION_BUCKET_NAME="${PROJECT_ID}-notifiers-config"
  DESTINATION_BUCKET_URI="gs://${DESTINATION_BUCKET_NAME}"
  DESTINATION_CONFIG_PATH="${DESTINATION_BUCKET_URI}/${SOURCE_CONFIG_BASENAME}"
  SOURCE_TEMPLATE_BASENAME=$(basename "${SOURCE_TEMPLATE_PATH}")
  DESTINATION_TEMPLATE_PATH="${DESTINATION_BUCKET_URI}/${SOURCE_TEMPLATE_BASENAME}"
  IMAGE_PATH="us-east1-docker.pkg.dev/gcb-release/cloud-build-notifiers/${NOTIFIER_TYPE}:latest"
  SERVICE_NAME="${NOTIFIER_TYPE}-notifier"
  SUBSCRIPTION_NAME="${NOTIFIER_TYPE}-subscription"
  INVOKER_SA="cloud-run-pubsub-invoker@${PROJECT_ID}.iam.gserviceaccount.com"
  PUBSUB_SA="service-${PROJECT_NUMBER}@gcp-sa-pubsub.iam.gserviceaccount.com"

  # Edge case: pubsub subscriptions cannot start with certain strings.
  # https://cloud.google.com/pubsub/docs/admin#resource_names
  if [ "${NOTIFIER_TYPE}" = "googlechat" ]; then
    SUBSCRIPTION_NAME="sub-${SUBSCRIPTION_NAME}"
  fi

  # Turn on command echoing after all of the variables have been set so we
  # don't log spam unnecessarily.
  set -x

  check_apis_enabled

  if [ -n "${SECRET_NAME}" ]; then
    add_secret_name_accessor_permission
  fi

  upload_config
  deploy_notifier
  SERVICE_URL=$(gcloud run services describe "${SERVICE_NAME}" \
    --format="value(status.url)")
  add_sa_token_creator_permission
  create_invoker_sa
  add_invoker_permission
  create_pubsub_topic
  check_pubsub_topic
  create_pubsub_subscription
  check_pubsub_subscription

  echo "** NOTIFIER SETUP COMPLETE **" 1>&2
}

fail() {
  echo "$*" 1>&2
  exit 1
}

check_apis_enabled() {
  if [ -n "${SECRET_NAME}" ]; then
    REQUIRED_SERVICES+=('secretmanager.googleapis.com')
  fi
  # Use config.name so that we only have to use the API URLs and don't have to
  # be clever with whitespace and matching. SERVICES is just a string
  # containing all of the enabled API URLs separated by spaces.
  SERVICES=$(gcloud services list --enabled --format='value(config.name)')
  for API in "${REQUIRED_SERVICES[@]}"; do
    [[ "${SERVICES}" =~ ${API} ]] || fail "please enable the '${API}' API"
  done
}

add_secret_name_accessor_permission() {
  gcloud secrets add-iam-policy-binding "${SECRET_NAME}" \
    --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" ||
    fail "failed to set up secret access"
}

upload_config() {
  # We allow this `mb` command to error since we rely on the `cp` command hard-
  # erroring if there's an actual problem (since `mb` fails if the bucket
  # already exists).
  gsutil mb -l asia1 "${DESTINATION_BUCKET_URI}"

  gsutil cp "${SOURCE_CONFIG_PATH}" "${DESTINATION_CONFIG_PATH}" ||
    fail "failed to copy config to GCS"

  if [ ! -z "${SOURCE_TEMPLATE_PATH}" ]; then
    gsutil cp "${SOURCE_TEMPLATE_PATH}" "${DESTINATION_TEMPLATE_PATH}" ||
      fail "failed to copy template to GCS"
  fi

}

deploy_notifier() {
  gcloud run services update "${SERVICE_NAME}" \
    --image="${IMAGE_PATH}" \
    --max-instances=1 \
    --update-env-vars="CONFIG_PATH=${DESTINATION_CONFIG_PATH},PROJECT_ID=${PROJECT_ID}" ||
    fail "failed to deploy notifier service -- check service logs for configuration error"
}

add_sa_token_creator_permission() {
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member="serviceAccount:${PUBSUB_SA}" \
    --role="roles/iam.serviceAccountTokenCreator"
}

create_invoker_sa() {
  gcloud iam service-accounts create cloud-run-pubsub-invoker \
    --display-name "Cloud Run Pub/Sub Invoker"
}

add_invoker_permission() {
  gcloud run services add-iam-policy-binding "${SERVICE_NAME}" \
    --member="serviceAccount:${INVOKER_SA}" \
    --role=roles/run.invoker
}

create_pubsub_topic() {
  gcloud pubsub topics create cloud-builds
}

check_pubsub_topic() {
  gcloud pubsub topics describe cloud-builds ||
    fail "expected the notifier Pub/Sub topic cloud-builds to exist"
}

create_pubsub_subscription() {
  gcloud pubsub subscriptions create "${SUBSCRIPTION_NAME}" \
    --topic=cloud-builds \
    --push-endpoint="${SERVICE_URL}" \
    --push-auth-service-account="${INVOKER_SA}" \
    --expiration-period="never"
}

check_pubsub_subscription() {
  gcloud pubsub subscriptions describe "${SUBSCRIPTION_NAME}" ||
    fail "expected the notifier Pub/Sub subscription to exist"
}

main "$@"

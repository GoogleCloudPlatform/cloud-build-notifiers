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

./setup.sh <notifier-type> <local-config-path> [-b destination-bucket-name] [-s secret-name] [-t local-template-path]

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

  DESTINATION_BUCKET_NAME=""
  SECRET_NAME=""
  SOURCE_TEMPLATE_PATH=""

  while [ $OPTIND -le "$#" ]
  do
    if getopts b:s:t: option
    then
        case $option
        in
            b) DESTINATION_BUCKET_NAME="$OPTARG";;
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
  if [ -z "${PROJECT_NUMBER}" ]; then
    fail "could not get project number for project ${PROJECT_ID}"
  fi

  # Region, from Cloud Run configuration
  REGION=$(gcloud config get-value run/region)
  if [ "${REGION}" = "" ] || [ "${REGION}" = "(unset)" ]; then
    fail "default cloud run region not set; run \"gcloud config set run/region <region>\""
  fi

  if [ -z "${DESTINATION_BUCKET_NAME}" ]; then
    RANDOM_SUFFIX=$(generate_random_suffix) ||
      fail "could not generate random suffix for bucket name"
    if [ -z "${RANDOM_SUFFIX}" ]; then
      fail "could not generate random suffix for bucket name"
    fi
    DESTINATION_BUCKET_NAME="notifiers-config-${RANDOM_SUFFIX}"
  else
    DESTINATION_BUCKET_NAME="${DESTINATION_BUCKET_NAME#gs://}"
    DESTINATION_BUCKET_NAME="${DESTINATION_BUCKET_NAME%/}"
  fi

  REQUIRED_SERVICES=('cloudbuild.googleapis.com' 'run.googleapis.com' 'pubsub.googleapis.com')
  SOURCE_CONFIG_BASENAME=$(basename "${SOURCE_CONFIG_PATH}")
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
  echo "Config bucket: ${DESTINATION_BUCKET_URI}" 1>&2
  echo "Config: ${DESTINATION_CONFIG_PATH}" 1>&2
  echo "re-run with -b ${DESTINATION_BUCKET_NAME} to update this notifier" 1>&2
}

fail() {
  echo "$*" 1>&2
  exit 1
}

generate_random_suffix() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
  elif [ -r /proc/sys/kernel/random/uuid ]; then
    tr -d '-' < /proc/sys/kernel/random/uuid | tr '[:upper:]' '[:lower:]'
  elif command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr -d '-' | tr '[:upper:]' '[:lower:]'
  else
    return 1
  fi
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

verify_bucket_owner() {
  BUCKET_PROJECT_NUMBER=$(gcloud storage buckets describe "${DESTINATION_BUCKET_URI}" \
    --raw --format="value(projectNumber)") ||
    fail "failed to describe bucket ${DESTINATION_BUCKET_URI}"

  if [ -z "${BUCKET_PROJECT_NUMBER}" ]; then
    fail "could not get project number for bucket ${DESTINATION_BUCKET_URI}"
  fi

  if [ "${BUCKET_PROJECT_NUMBER}" != "${PROJECT_NUMBER}" ]; then
    fail "bucket ${DESTINATION_BUCKET_URI} is not owned by project ${PROJECT_ID} (expected project number ${PROJECT_NUMBER}, got ${BUCKET_PROJECT_NUMBER})"
  fi
}

upload_config() {
  if ! gcloud storage buckets describe "${DESTINATION_BUCKET_URI}" >/dev/null 2>&1; then
    gcloud storage buckets create "${DESTINATION_BUCKET_URI}" \
      --project="${PROJECT_ID}" \
      --uniform-bucket-level-access ||
      fail "failed to create bucket ${DESTINATION_BUCKET_URI}"
  fi

  verify_bucket_owner

  if [ ! -z "${SOURCE_TEMPLATE_PATH}" ]; then
    TEMP_CONFIG_PATH=$(mktemp) ||
      fail "failed to create temporary config file"
    sed "s|\(uri:[[:space:]]*['\"]*\)gs://[^'\"[:space:]]*|\1${DESTINATION_TEMPLATE_PATH}|" \
      "${SOURCE_CONFIG_PATH}" > "${TEMP_CONFIG_PATH}" || {
      rm -f "${TEMP_CONFIG_PATH}"
      fail "failed to update template URI in config"
    }
    gcloud storage cp "${TEMP_CONFIG_PATH}" "${DESTINATION_CONFIG_PATH}" || {
      rm -f "${TEMP_CONFIG_PATH}"
      fail "failed to copy config to GCS"
    }
    rm -f "${TEMP_CONFIG_PATH}"

    gcloud storage cp "${SOURCE_TEMPLATE_PATH}" "${DESTINATION_TEMPLATE_PATH}" ||
      fail "failed to copy template to GCS"
  else
    gcloud storage cp "${SOURCE_CONFIG_PATH}" "${DESTINATION_CONFIG_PATH}" ||
      fail "failed to copy config to GCS"
  fi

}

deploy_notifier() {
  gcloud run deploy "${SERVICE_NAME}" \
    --image="${IMAGE_PATH}" \
    --no-allow-unauthenticated \
    --update-env-vars="CONFIG_PATH=${DESTINATION_CONFIG_PATH},PROJECT_ID=${PROJECT_ID},PROJECT_NUMBER=${PROJECT_NUMBER}" ||
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
    --push-auth-service-account="${INVOKER_SA}"
}

check_pubsub_subscription() {
  gcloud pubsub subscriptions describe "${SUBSCRIPTION_NAME}" ||
    fail "expected the notifier Pub/Sub subscription to exist"
}

main "$@"

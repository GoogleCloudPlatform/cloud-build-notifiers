# Cloud Build Notifier Sample

This sample shows how to create a simple Cloud Build notifier.

Use it with the [Creating your own notifer tutorial](https://cloud.google.com/cloud-build/docs/configuring-notifications/create-notifier).

## Upload the config to a Google Cloud Storage bucket 

First create a bucket.

```sh
export BUCKET_NAME=your_bucket_name
gsutil mb gs://${BUCKET_NAME}/
```

Then upload the provided notifier config to the bucket.

```sh
export CONFIG_FILE_NAME=notifier_config.yaml
gsutil cp ${CONFIG_FILE_NAME} gs://${BUCKET_NAME}/${CONFIG_FILE_NAME}
```

## Build and Deploy

Commands should be run from this directory: `samples/logging-sample`.

```sh
gcloud builds submit .  --substitutions=_CONFIG_PATH=gs://${BUCKET_NAME}/${CONFIG_FILE_NAME}
```

Running this `cloudbuild.yaml` will create a Cloud Run service.  You can see it in the console [here](https://console.cloud.google.com/run).


## Set up permissions 
Follow the [create your own notifer tutorial](https://cloud.google.com/cloud-build/docs/configuring-notifications/create-notifier#configuring_notifications) to set up the permissions. Specifically, PubSub must be able to create tokens and invoke the Cloud Run service.


## Try it out!

Test the output by running the `success.yaml` and `failure.yaml`.  The failure and success should trigger the Cloud Run service and be reflected in the Cloud Run logs.  The `success.yaml` and `failure.yaml` files are available in the directory one level up from this one.

```sh
gcloud builds submit --config ../success.yaml --no-source
gcloud builds submit --config ../failure.yaml --no-source
```

#!/bin/bash
set -e

# Pick up PROJECT_ID from env variable, but default to deploy-demo-test
PROJECT_ID=${PROJECT_ID:-duet01}
REGION=${REGION:-us-west1}
SERVICE_NAME=${SERVICE_NAME:-scion-docs}

echo "Deploying Scion Documentation Site..."
echo "Project ID: $PROJECT_ID"
echo "Region:     $REGION"
echo "Service:    $SERVICE_NAME"

# Submit to Cloud Build
# We pass the project explicitly to gcloud
# We calculate a short SHA for tagging if in a git repo
GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "latest")
REGISTRY="${REGION}-docker.pkg.dev/${PROJECT_ID}/scion-images"

gcloud builds submit \
  --async \
  --project "$PROJECT_ID" \
  --config cloudbuild.yaml \
  --substitutions="_SERVICE_NAME=$SERVICE_NAME,_REGION=$REGION,_GIT_SHA=$GIT_SHA,_REGISTRY=$REGISTRY" \
  .

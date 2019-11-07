#!/bin/bash

IMAGE=autonomy/installer
TAG=latest

TOKEN=$(curl -s "https://auth.docker.io/token?scope=repository:$IMAGE:pull&service=registry.docker.io" | jq -r .token)
echo $TOKEN
CONFIG_DIGEST=$(curl -s -H"Accept: application/vnd.docker.distribution.manifest.v2+json" -H"Authorization: Bearer $TOKEN" "https://registry-1.docker.io/v2/$IMAGE/manifests/$TAG" | jq -r .config.digest)
echo $CONFIG_DIGEST

LABELS=$(curl -sL -H"Authorization: Bearer $TOKEN" "https://registry-1.docker.io/v2/$IMAGE/blobs/$CONFIG_DIGEST" | jq -r .config.Labels)
echo $LABELS

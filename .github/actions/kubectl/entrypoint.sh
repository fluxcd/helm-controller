#!/bin/bash
set -e

kubectl_ver=1.18.3 && \
curl -sL https://storage.googleapis.com/kubernetes-release/release/v${kubectl_ver}/bin/linux/amd64/kubectl > kubectl

mkdir -p $GITHUB_WORKSPACE/bin
cp ./kubectl $GITHUB_WORKSPACE/bin
chmod +x $GITHUB_WORKSPACE/bin/kubectl

$GITHUB_WORKSPACE/bin/kubectl version --client

echo "::add-path::$GITHUB_WORKSPACE/bin"
echo "::add-path::$RUNNER_WORKSPACE/$(basename $GITHUB_REPOSITORY)/bin"

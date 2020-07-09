#!/bin/bash
set -e

helm_ver=3.2.4 && \
helm_url=https://get.helm.sh && \
curl -sL ${helm_url}/helm-v${helm_ver}-linux-amd64.tar.gz | \
tar xz

mkdir -p $GITHUB_WORKSPACE/bin
cp ./linux-amd64/helm $GITHUB_WORKSPACE/bin
chmod +x $GITHUB_WORKSPACE/bin/helm

$GITHUB_WORKSPACE/bin/helm version

echo "::add-path::$GITHUB_WORKSPACE/bin"
echo "::add-path::$RUNNER_WORKSPACE/$(basename $GITHUB_REPOSITORY)/bin"

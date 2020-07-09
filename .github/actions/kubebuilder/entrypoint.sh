#!/bin/sh -l

VERSION=2.3.1

curl -sL https://go.kubebuilder.io/dl/${VERSION}/linux/amd64 | tar -xz -C /tmp/

mkdir -p $GITHUB_WORKSPACE/kubebuilder
mv /tmp/kubebuilder_${VERSION}_linux_amd64/* $GITHUB_WORKSPACE/kubebuilder/
ls -lh $GITHUB_WORKSPACE/kubebuilder/bin

echo "::add-path::$GITHUB_WORKSPACE/kubebuilder/bin"
echo "::add-path::$RUNNER_WORKSPACE/$(basename $GITHUB_REPOSITORY)/kubebuilder/bin"

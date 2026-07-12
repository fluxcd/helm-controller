#!/usr/bin/env bash

echo ">>> Test samples"
kubectl create ns samples
kubectl -n samples apply -f config/samples
kubectl -n samples wait hr/podinfo-ocirepository --for=condition=ready --timeout=4m
kubectl -n samples wait hr/podinfo-gitrepository --for=condition=ready --timeout=4m
kubectl -n samples wait hr/podinfo-helmrepository --for=condition=ready --timeout=4m
kubectl delete ns samples
echo ">>> Run smoke test"
kubectl -n helm-system apply -f config/testdata/podinfo
kubectl -n helm-system wait helmreleases/podinfo --for=condition=ready --timeout=4m

# Inventory tracking enables drift detection and garbage collection.
# Ensure it captures managed objects from the Helm release.
INVENTORY=$(kubectl -n helm-system get helmrelease/podinfo -o jsonpath='{.status.inventory.entries}')
INVENTORY_COUNT=$(echo "$INVENTORY" | jq 'length')
if [ "$INVENTORY_COUNT" -lt 1 ]; then
  echo "Expected inventory entries, got $INVENTORY_COUNT"
  exit 1
fi
# Deployment is a primary workload resource; its presence confirms
# that the inventory correctly tracks resources from the rendered manifests.
if ! echo "$INVENTORY" | jq -e '.[] | select(.id | contains("_Deployment"))' > /dev/null; then
  echo "Expected Deployment in inventory"
  echo "Inventory: $INVENTORY"
  exit 1
fi

kubectl -n helm-system wait helmreleases/podinfo-git --for=condition=ready --timeout=4m
kubectl -n helm-system wait helmreleases/podinfo-oci --for=condition=ready --timeout=4m
kubectl -n helm-system delete -f config/testdata/podinfo
echo ">>> Run Job with TTL test"
# This test verifies that the wait logic correctly handles Jobs with
# ttlSecondsAfterFinished that get garbage-collected after completion.
# Without the fix, the wait would fail with NotFound error.
kubectl -n helm-system apply -f config/testdata/job-ttl
kubectl -n helm-system wait helmreleases/job-ttl --for=condition=ready --timeout=4m
kubectl -n helm-system delete -f config/testdata/job-ttl
echo ">>> Run dependency tests"
kubectl -n helm-system apply -f config/testdata/dependencies
kubectl -n helm-system wait helmreleases/backend --for=condition=ready --timeout=4m
kubectl -n helm-system wait helmreleases/frontend --for=condition=ready --timeout=4m
kubectl -n helm-system delete -f config/testdata/dependencies
echo ">>> Run values test"
kubectl -n helm-system apply -f config/testdata/valuesfrom
kubectl -n helm-system wait helmreleases/valuesfrom --for=condition=ready --timeout=4m

RESULT=$(helm -n helm-system get values valuesfrom)
EXPECTED=$(cat config/testdata/valuesfrom/golden.txt)
if [ "$RESULT" != "$EXPECTED" ]; then
  echo -e "$RESULT\n\ndoes not equal\n\n$EXPECTED"
  exit 1
fi

 kubectl -n helm-system delete -f config/testdata/valuesfrom
echo ">>> Run target namespace test"
kubectl -n helm-system apply -f config/testdata/targetnamespace
kubectl -n helm-system wait helmreleases/targetnamespace --for=condition=ready --timeout=4m

# Confirm release in "default" namespace
kubectl -n default get deployment default-targetnamespace-podinfo

kubectl -n helm-system delete -f config/testdata/targetnamespace
echo ">>> Run install create target namespace test"
kubectl -n helm-system apply -f config/testdata/install-create-target-ns
kubectl -n helm-system wait helmreleases/install-create-target-ns --for=condition=ready --timeout=4m

# Confirm release in "install-create-target-ns" namespace
kubectl -n install-create-target-ns get deployment install-create-target-ns-install-create-target-ns-podinfo

kubectl -n helm-system delete -f config/testdata/install-create-target-ns
echo ">>> Run install from helmChart test"
kubectl -n helm-system apply -f config/testdata/install-from-hc-source
kubectl -n helm-system wait helmreleases/podinfo-from-hc --for=condition=ready --timeout=4m
kubectl -n helm-system delete -f config/testdata/install-from-hc-source
echo ">>> Run install from ocirepo test"
kubectl -n helm-system apply -f config/testdata/install-from-ocirepo-source
kubectl -n helm-system wait helmreleases/podinfo-from-ocirepo --for=condition=ready --timeout=4m
kubectl -n helm-system delete -f config/testdata/install-from-ocirepo-source
echo ">>> Run post-renderer-kustomize test"
kubectl -n helm-system apply -f config/testdata/post-renderer-kustomize
kubectl -n helm-system wait helmreleases/post-renderer-kustomize --for=condition=ready --timeout=4m
RESULT=$(kubectl get deployment -n helm-system mypodinfo -o jsonpath='{.metadata.labels.xxxx}')
if [ "$RESULT" != "yyyy" ]; then
  echo -e "$RESULT\n\ndoes not equal\n\nyyyy"
  exit 1
fi
RESULT=$(kubectl get deployment -n helm-system mypodinfo -o jsonpath='{.metadata.labels.yyyy}')
if [ "$RESULT" != "xxxx" ]; then
  echo -e "$RESULT\n\ndoes not equal\n\nxxxx"
  exit 1
fi
kubectl -n helm-system delete -f config/testdata/post-renderer-kustomize

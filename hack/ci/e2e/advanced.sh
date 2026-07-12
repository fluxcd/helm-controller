#!/usr/bin/env bash

echo ">>> Run impersonation tests"
kubectl apply -f config/testdata/impersonation
kubectl -n impersonation wait helmreleases/podinfo --for=condition=ready --timeout=2m
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n impersonation get helmrelease/podinfo-fail -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'
echo ">>> Run delete-ns tests"
kubectl apply -f config/testdata/delete-ns
kubectl -n delete-ns wait helmreleases/podinfo --for=condition=ready --timeout=2m
kubectl delete ns delete-ns 1>/dev/null 2>&1 &
echo -n ">>> Waiting for namespace to be deleted"
if kubectl wait --for=delete namespace delete-ns --timeout=5m; then
  echo ' Namespace deleted successfully'
else
  echo ' Timed out waiting for namespace to be deleted'
  kubectl get all -n delete-ns
  exit 1
fi

echo ">>> Bootstrap Tests Using Local Helm Chart"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  if [ "$GITHUB_EVENT_NAME" = "pull_request" ]; then
    TYPE=branch
    REF="${GITHUB_HEAD_REF}"
  else
    REF=${GITHUB_REF}
    if echo "$REF" | grep 'refs/tags/'; then
      TYPE=tag
      REF=${REF#refs/tags/}
    else
      TYPE=branch
      REF=${REF#refs/heads/}
    fi
  fi
  echo "REF=$REF of type $TYPE"
  echo "helm install --namespace default --set $TYPE=$REF --set url=https://github.com/$GITHUB_REPOSITORY this config/testdata/charts/crds/bootstrap"
  helm install --namespace default --set $TYPE=$REF --set url=https://github.com/$GITHUB_REPOSITORY this config/testdata/charts/crds/bootstrap
  kubectl -n default apply -f config/testdata/crds-upgrade/init
  kubectl -n default wait helmreleases/crds-upgrade-test --for=condition=ready --timeout=2m
fi
echo ">>> CRDs Upgrade Test Create"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  kubectl -n default apply -f config/testdata/crds-upgrade/create
  kubectl -n default wait helmreleases/crds-upgrade-test --for=condition=ready --timeout=2m
fi
echo ">>> CRDs Upgrade Test CreateReplace"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  kubectl -n default apply -f config/testdata/crds-upgrade/create-replace
  kubectl -n default wait helmreleases/crds-upgrade-test --for=condition=ready --timeout=2m
fi
echo ">>> Run upgrade with chart name changed and default ChartNameChangeStrategy"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  test_name=upgrade-chart-name-change
  kubectl -n helm-system apply -f config/testdata/$test_name/install.yaml
  echo -n ">>> Waiting for expected conditions"
  count=0
  until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .Ready=="True"' )" ]; do
    echo -n '.'
    sleep 5
    count=$((count + 1))
    if [[ ${count} -eq 24 ]]; then
      echo ' No more retries left!'
      exit 1
    fi
  done
  echo ' done'

  # Validate release was installed.
  HISTORY=$(helm -n helm-system history -o json $test_name)
  REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
  if [ "$REVISION_COUNT" != 1 ]; then
    echo -e "Unexpected revision count: $REVISION_COUNT"
    exit 1
  fi
  LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
  if [ "$LAST_REVISION_STATUS" != "deployed" ]; then
    echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
    exit 1
  fi

  kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml

  kubectl -n helm-system wait helmreleases/$test_name --for=condition=ready --timeout=4m

  # Validate release was uninstalled/reinstalled.
  HISTORY=$(helm -n helm-system history -o json $test_name)
  REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
  if [ "$REVISION_COUNT" != 1 ]; then
    echo -e "Unexpected revision count: $REVISION_COUNT"
    exit 1
  fi
  LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
  if [ "$LAST_REVISION_STATUS" != "deployed" ]; then
    echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
    exit 1
  fi

  kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
fi
echo ">>> Run upgrade with chart name changed and reinstall ChartNameChangeStrategy"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  test_name=upgrade-chart-name-change
  kubectl -n helm-system apply -f config/testdata/$test_name/install.yaml
  echo -n ">>> Waiting for expected conditions"
  count=0
  until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .Ready=="True"' )" ]; do
    echo -n '.'
    sleep 5
    count=$((count + 1))
    if [[ ${count} -eq 24 ]]; then
      echo ' No more retries left!'
      exit 1
    fi
  done
  echo ' done'

  # Validate release was installed.
  HISTORY=$(helm -n helm-system history -o json $test_name)
  REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
  if [ "$REVISION_COUNT" != 1 ]; then
    echo -e "Unexpected revision count: $REVISION_COUNT"
    exit 1
  fi
  LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
  if [ "$LAST_REVISION_STATUS" != "deployed" ]; then
    echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
    exit 1
  fi

  kubectl -n helm-system apply -f config/testdata/$test_name/upgrade-reinstall.yaml
  kubectl -n helm-system wait helmreleases/$test_name --for=condition=ready --timeout=4m

  # Validate release was uninstalled/reinstalled.
  HISTORY=$(helm -n helm-system history -o json $test_name)
  REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
  if [ "$REVISION_COUNT" != 1 ]; then
    echo -e "Unexpected revision count: $REVISION_COUNT"
    exit 1
  fi
  LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
  if [ "$LAST_REVISION_STATUS" != "deployed" ]; then
    echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
    exit 1
  fi

  kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
fi
echo ">>> Run upgrade with chart name changed and InPlaceUpdate ChartNameChangeStrategy"
PR_HEAD_REPO=$(jq -r '.pull_request.head.repo.full_name // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || true)
if [[ "$GITHUB_REF" == refs/tags/* ]] || [[ "$GITHUB_REF" == refs/heads/* ]] || { [[ "$GITHUB_EVENT_NAME" == "pull_request" ]] && [[ "$PR_HEAD_REPO" == "$GITHUB_REPOSITORY" ]]; }; then
  test_name=upgrade-chart-name-change
  kubectl -n helm-system apply -f config/testdata/$test_name/install.yaml
  echo -n ">>> Waiting for expected conditions"
  count=0
  until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .Ready=="True"' )" ]; do
    echo -n '.'
    sleep 5
    count=$((count + 1))
    if [[ ${count} -eq 24 ]]; then
      echo ' No more retries left!'
      exit 1
    fi
  done
  echo ' done'

  # Validate release was installed.
  HISTORY=$(helm -n helm-system history -o json $test_name)
  REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
  if [ "$REVISION_COUNT" != 1 ]; then
    echo -e "Unexpected revision count: $REVISION_COUNT"
    exit 1
  fi
  LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
  if [ "$LAST_REVISION_STATUS" != "deployed" ]; then
    echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
    exit 1
  fi

  kubectl -n helm-system apply -f config/testdata/$test_name/upgrade-inplace.yaml

  # Wait for the upgrade to complete (revision count should be 2).
  echo -n ">>> Waiting for upgrade"
  count=0
  until [ '2' == "$(helm -n helm-system history -o json $test_name 2>/dev/null | jq 'length')" ]; do
    echo -n '.'
    sleep 5
    count=$((count + 1))
    if [[ ${count} -eq 24 ]]; then
      echo ' No more retries left!'
      exit 1
    fi
  done
  echo ' done'

  kubectl -n helm-system wait helmreleases/$test_name --for=condition=ready --timeout=4m

  kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
fi

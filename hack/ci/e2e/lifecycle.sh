#!/usr/bin/env bash

echo ">>> Run install fail test"
test_name=install-fail
kubectl -n helm-system apply -f config/testdata/$test_name
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was installed and not uninstalled.
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system delete -f config/testdata/$test_name
echo ">>> Run install test fail test"
test_name=install-test-fail
kubectl -n helm-system apply -f config/testdata/$test_name
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .TestSuccess=="False" and .Ready=="False"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was installed and not uninstalled.
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system delete -f config/testdata/$test_name
echo ">>> Run install test fail ignore test"
test_name=install-test-fail-ignore
kubectl -n helm-system apply -f config/testdata/$test_name
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .TestSuccess=="False" and .Ready=="True"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was installed and not uninstalled.
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system delete -f config/testdata/$test_name
echo ">>> Run install fail with remediation test"
test_name=install-fail-remediate
kubectl -n helm-system apply -f config/testdata/$test_name
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" and .Remediated=="True"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Ensure release was uninstalled.
RELEASE_STATUS=$(helm -n helm-system history $test_name -o json | jq -r 'if length == 1 then .[0].status else empty end')
if [ "$RELEASE_STATUS" != "uninstalled" ]; then
  echo -e "Unexpected release status: $RELEASE_STATUS"
  exit 1
fi

kubectl -n helm-system delete -f config/testdata/$test_name
helm -n helm-system delete $test_name
echo ">>> Run install fail with retry test"
test_name=install-fail-retry
kubectl -n helm-system apply -f config/testdata/$test_name
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.installFailures == 2 and ( .status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" and .Stalled=="True" )' )" ]; do
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system delete -f config/testdata/$test_name
echo ">>> Run upgrade fail test"
test_name=upgrade-fail
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" and .Stalled=="True"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was upgraded and not rolled back.
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 2 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
echo ">>> Run upgrade test fail test"
test_name=upgrade-test-fail
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="True" and .TestSuccess=="False" and .Ready=="False" and .Stalled=="True"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was upgraded and not rolled back.
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 2 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
echo ">>> Run upgrade fail with remediation test"
test_name=upgrade-fail-remediate
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" and .Remediated=="True"' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was upgraded and then rolled back.
HISTORY=$(helm -n helm-system history -o json $test_name)
REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
if [ "$REVISION_COUNT" != 3 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
fi
LAST_REVISION_DESCRIPTION=$(echo "$HISTORY" | jq -r 'last | .description')
if [ "$LAST_REVISION_DESCRIPTION" != "Rollback to 1" ]; then
  echo -e "Unexpected last revision description: $LAST_REVISION_DESCRIPTION"
  exit 1
fi

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
echo ">>> Run upgrade fail retry test"
test_name=upgrade-fail-retry
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.upgradeFailures == 2 and ( .status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" )' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was upgraded and rolled back twice.
HISTORY=$(helm -n helm-system history -o json $test_name)
REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
if [ "$REVISION_COUNT" != 5 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
fi
LAST_REVISION_DESCRIPTION=$(echo "$HISTORY" | jq -r 'last | .description')
if [ "$LAST_REVISION_DESCRIPTION" != "Rollback to 3" ]; then
  echo -e "Unexpected last revision description: $LAST_REVISION_DESCRIPTION"
  exit 1
fi

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
echo ">>> Run upgrade from ocirepo source"
test_name=upgrade-from-ocirepo-source
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
REVISION_COUNT=$(helm -n helm-system history -o json $test_name | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi

kubectl -n helm-system apply -f config/testdata/$test_name/upgrade.yaml
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

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml
echo ">>> Run upgrade fail with uninstall remediation strategy test"
test_name=upgrade-fail-remediate-uninstall
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
echo -n ">>> Waiting for expected conditions"
count=0
until [ 'true' == "$( kubectl -n helm-system get helmrelease/$test_name -o json | jq '.status.upgradeFailures == 1 and .status.installFailures == 1 and ( .status.conditions | map( { (.type): .status } ) | add | .Released=="False" and .Ready=="False" )' )" ]; do
  echo -n '.'
  sleep 5
  count=$((count + 1))
  if [[ ${count} -eq 24 ]]; then
    echo ' No more retries left!'
    exit 1
  fi
done
echo ' done'

# Validate release was uninstalled/reinstalled.
HISTORY=$(helm -n helm-system history -o json $test_name)
REVISION_COUNT=$(echo "$HISTORY" | jq 'length')
if [ "$REVISION_COUNT" != 1 ]; then
  echo -e "Unexpected revision count: $REVISION_COUNT"
  exit 1
fi
LAST_REVISION_STATUS=$(echo "$HISTORY" | jq -r 'last | .status')
if [ "$LAST_REVISION_STATUS" != "failed" ]; then
  echo -e "Unexpected last revision status: $LAST_REVISION_STATUS"
  exit 1
fi

kubectl delete -n helm-system -f config/testdata/$test_name/install.yaml

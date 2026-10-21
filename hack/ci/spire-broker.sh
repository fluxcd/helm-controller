#!/usr/bin/env bash
# Installs SPIRE with the SPIFFE Broker API and OIDC discovery provider in the
# current kubectl context. Expects an already-running single-node kind cluster.
#
# Required environment:
#   SPIRE_NAMESPACE, SPIRE_TRUST_DOMAIN, SPIRE_SERVER_IMAGE, SPIRE_AGENT_IMAGE,
#   SPIRE_OIDC_IMAGE, SPIRE_CLUSTER, SPIRE_OIDC_ISSUER, BROKER_SA,
#   NS, APP_NS, JWT_OBJ_TYPE, JWT_OBJ_NAME, JWT_SPIFFE_ID
#
# Optional environment:
#   SPIRE_JWT_ISSUER (defaults to SPIRE_OIDC_ISSUER) is the JWT issuer URL
#   embedded in JWT-SVIDs. Set it when the trusted OIDC endpoint differs from
#   the local discovery provider, e.g. an HTTPS endpoint on another port.
set -euo pipefail

install_spire_broker() {
log "Installing SPIRE with the SPIFFE Broker API and OIDC discovery"
##############################################################################
kubectl create namespace "${SPIRE_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: spire-server
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["get", "create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spire-server
subjects:
  - kind: ServiceAccount
    name: spire-server
    namespace: ${SPIRE_NAMESPACE}
roleRef:
  kind: ClusterRole
  name: spire-server
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    resourceNames: ["spire-bundle"]
    verbs: ["get", "patch"]
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["create"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["create", "update", "get"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
subjects:
  - kind: ServiceAccount
    name: spire-server
    namespace: ${SPIRE_NAMESPACE}
roleRef:
  kind: Role
  name: spire-server
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: spire-bundle
  namespace: ${SPIRE_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
data:
  server.conf: |
    server {
      bind_address = "0.0.0.0"
      bind_port = "8081"
      trust_domain = "${SPIRE_TRUST_DOMAIN}"
      data_dir = "/run/spire/data"
      log_level = "INFO"
      default_x509_svid_ttl = "1h"
      ca_ttl = "12h"
      jwt_issuer = "${SPIRE_JWT_ISSUER:-${SPIRE_OIDC_ISSUER}}"
    }

    plugins {
      DataStore "sql" {
        plugin_data {
          database_type = "sqlite3"
          connection_string = "/run/spire/data/datastore.sqlite3"
        }
      }

      NodeAttestor "k8s_psat" {
        plugin_data {
          clusters = {
            "broker-cluster" = {
              service_account_allow_list = ["${SPIRE_NAMESPACE}:spire-agent"]
            }
          }
        }
      }

      KeyManager "disk" {
        plugin_data {
          keys_path = "/run/spire/data/keys.json"
        }
      }

      BundlePublisher "k8s_configmap" {
        plugin_data {
          clusters = {
            "broker-cluster" = {
              configmap_name = "spire-bundle"
              configmap_key  = "bundle.crt"
              namespace      = "${SPIRE_NAMESPACE}"
              format         = "pem"
            }
          }
        }
      }
    }

    health_checks {
      listener_enabled = true
      bind_address = "0.0.0.0"
      bind_port = 8080
      live_path = "/live"
      ready_path = "/ready"
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
  labels:
    app: spire-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: spire-server
  template:
    metadata:
      labels:
        app: spire-server
    spec:
      serviceAccountName: spire-server
      containers:
        - name: spire-server
          image: ${SPIRE_SERVER_IMAGE}
          args: ["-config", "/run/spire/config/server.conf"]
          ports:
            - containerPort: 8081
          volumeMounts:
            - name: spire-config
              mountPath: /run/spire/config
              readOnly: true
          livenessProbe:
            httpGet:
              path: /live
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: spire-config
          configMap:
            name: spire-server
---
apiVersion: v1
kind: Service
metadata:
  name: spire-server
  namespace: ${SPIRE_NAMESPACE}
spec:
  ports:
    - name: grpc
      port: 8081
      targetPort: 8081
  selector:
    app: spire-server
EOF
kubectl -n "${SPIRE_NAMESPACE}" rollout status deployment/spire-server --timeout=5m

NODE_UID="$(kubectl get nodes -o jsonpath='{.items[0].metadata.uid}')"
AGENT_ID="spiffe://${SPIRE_TRUST_DOMAIN}/spire/agent/k8s_psat/broker-cluster/${NODE_UID}"

create_entry() {
    local spiffe_id="$1"
    shift
    kubectl -n "${SPIRE_NAMESPACE}" exec deployment/spire-server -- \
        /opt/spire/bin/spire-server entry create \
            -spiffeID "${spiffe_id}" \
            -parentID "${AGENT_ID}" \
            "$@" >/dev/null
}

create_entry "spiffe://${SPIRE_TRUST_DOMAIN}/oidc-discovery-provider" \
    -selector "k8s:ns:${SPIRE_NAMESPACE}" \
    -selector "k8s:sa:spire-agent" \
    -selector "k8s:container-name:oidc-discovery-provider"
create_entry "spiffe://${SPIRE_TRUST_DOMAIN}/ns/${NS}/sa/${BROKER_SA}" \
    -selector "k8s:ns:${NS}" \
    -selector "k8s:sa:${BROKER_SA}"
create_entry "${JWT_SPIFFE_ID}" \
    -selector "k8s:resource:${JWT_OBJ_TYPE}" \
    -selector "k8s:namespace:${APP_NS}" \
    -selector "k8s:name:${JWT_OBJ_NAME}"

kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-agent
  namespace: ${SPIRE_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: spire-agent
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes", "nodes/proxy"]
    verbs: ["get", "list"]
  - apiGroups: ["source.toolkit.fluxcd.io"]
    resources: ["ocirepositories"]
    verbs: ["get", "list"]
  - apiGroups: ["image.toolkit.fluxcd.io"]
    resources: ["imagerepositories"]
    verbs: ["get", "list"]
  - apiGroups: ["helm.toolkit.fluxcd.io"]
    resources: ["helmreleases"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spire-agent
subjects:
  - kind: ServiceAccount
    name: spire-agent
    namespace: ${SPIRE_NAMESPACE}
roleRef:
  kind: ClusterRole
  name: spire-agent
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: spire-agent
  namespace: ${SPIRE_NAMESPACE}
data:
  agent.conf: |
    agent {
      data_dir = "/run/spire"
      log_level = "INFO"
      server_address = "spire-server"
      server_port = "8081"
      socket_path = "/run/spire/agent-sockets/api.sock"
      trust_bundle_path = "/run/spire/bundle/bundle.crt"
      trust_domain = "${SPIRE_TRUST_DOMAIN}"

      experimental {
        broker {
          bind_address = "0.0.0.0:8443"
          brokers = [
            {
              id = "spiffe://${SPIRE_TRUST_DOMAIN}/ns/${NS}/sa/${BROKER_SA}"
              allowed_reference_types = [
                {
                  type_url = "type.googleapis.com/spiffe.broker.KubernetesObjectReference"
                  allow_over_tcp = true
                },
              ]
            },
          ]
        }
      }
    }

    plugins {
      NodeAttestor "k8s_psat" {
        plugin_data {
          cluster = "broker-cluster"
        }
      }

      KeyManager "memory" {
        plugin_data {}
      }

      WorkloadAttestor "k8s" {
        plugin_data {
          skip_kubelet_verification = true
          node_name_env = "MY_NODE_NAME"

          experimental {
            broker {
              access_policy = "permissive"
              brokers = [
                {
                  id = "spiffe://${SPIRE_TRUST_DOMAIN}/ns/${NS}/sa/${BROKER_SA}"
                },
              ]
            }
          }
        }
      }
    }

    health_checks {
      listener_enabled = true
      bind_address = "0.0.0.0"
      bind_port = 8080
      live_path = "/live"
      ready_path = "/ready"
    }
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: spire-oidc
  namespace: ${SPIRE_NAMESPACE}
data:
  oidc.conf: |
    log_level = "INFO"
    domains = ["${SPIRE_CLUSTER}-control-plane"]
    insecure_addr = "0.0.0.0:8082"
    allow_insecure_scheme = true
    jwt_issuer = "${SPIRE_OIDC_ISSUER}"
    jwks_uri = "${SPIRE_OIDC_ISSUER}/keys"
    workload_api {
      socket_path = "/run/spire/agent-sockets/api.sock"
      trust_domain = "${SPIRE_TRUST_DOMAIN}"
    }
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: spire-agent
  namespace: ${SPIRE_NAMESPACE}
  labels:
    app: spire-agent
spec:
  selector:
    matchLabels:
      app: spire-agent
  template:
    metadata:
      labels:
        app: spire-agent
    spec:
      hostPID: true
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      serviceAccountName: spire-agent
      containers:
        - name: spire-agent
          image: ${SPIRE_AGENT_IMAGE}
          args: ["-config", "/run/spire/config/agent.conf"]
          env:
            - name: MY_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          ports:
            - name: broker
              containerPort: 8443
            - name: healthz
              containerPort: 8080
          volumeMounts:
            - name: spire-config
              mountPath: /run/spire/config
              readOnly: true
            - name: spire-bundle
              mountPath: /run/spire/bundle
              readOnly: true
            - name: spire-agent-socket
              mountPath: /run/spire/agent-sockets
            - name: spire-token
              mountPath: /var/run/secrets/tokens
          livenessProbe:
            httpGet:
              path: /live
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
        - name: oidc-discovery-provider
          image: ${SPIRE_OIDC_IMAGE}
          args: ["-config", "/run/spire/oidc/oidc.conf"]
          ports:
            - name: oidc
              containerPort: 8082
          readinessProbe:
            httpGet:
              path: /.well-known/openid-configuration
              port: 8082
              httpHeaders:
                - name: Host
                  value: ${SPIRE_CLUSTER}-control-plane
            initialDelaySeconds: 5
            periodSeconds: 5
          volumeMounts:
            - name: spire-oidc
              mountPath: /run/spire/oidc
              readOnly: true
            - name: spire-agent-socket
              mountPath: /run/spire/agent-sockets
      volumes:
        - name: spire-config
          configMap:
            name: spire-agent
        - name: spire-bundle
          configMap:
            name: spire-bundle
        - name: spire-oidc
          configMap:
            name: spire-oidc
        - name: spire-agent-socket
          hostPath:
            path: /run/spire/agent-sockets
            type: DirectoryOrCreate
        - name: spire-token
          projected:
            sources:
              - serviceAccountToken:
                  path: spire-agent
                  expirationSeconds: 7200
                  audience: spire-server
---
apiVersion: v1
kind: Service
metadata:
  name: spire-broker
  namespace: ${SPIRE_NAMESPACE}
spec:
  selector:
    app: spire-agent
  ports:
    - name: broker
      port: 8443
      targetPort: 8443
EOF
kubectl -n "${SPIRE_NAMESPACE}" rollout status daemonset/spire-agent --timeout=5m

##############################################################################
}

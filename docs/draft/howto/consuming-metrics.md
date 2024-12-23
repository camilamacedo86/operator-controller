# Consuming Metrics

!!! warning
Metrics endpoints and ports are available as an alpha release and are subject to change in future versions.
The following procedure is provided as an example for testing purposes. Do not depend on alpha features in production clusters.

Operator-Controller and CatalogD are configured to export metrics by default. The metrics are exposed on the `/metrics` endpoint of the respective services.

The metrics are secured by [RBAC policies][rbac-k8s-docs], requiring appropriate permissions for access.
By default, they are exposed over HTTPS, necessitating valid certificates for integration with services like Prometheus.
The following sections cover enabling metrics, validating access, and integrating with the [Prometheus Operator][prometheus-operator].

Below, you will learn how to enable the metrics, validate access, and integrate with [Prometheus Operator][prometheus-operator].

---

## Operator-Controller Metrics

### Step 1: Enable Access

To enable access to the Operator-Controller metrics, create a `ClusterRoleBinding` to
allow the Operator-Controller service account to access the metrics.

```shell
kubectl create clusterrolebinding operator-controller-metrics-binding \
   --clusterrole=operator-controller-metrics-reader \
   --serviceaccount=olmv1-system:operator-controller-controller-manager
```

### Step 2: Validate Access Manually

#### Create a Token and Extract Certificates

Generate a token for the service account and extract the required certificates:

```shell
TOKEN=$(kubectl create token operator-controller-controller-manager -n olmv1-system)
echo $TOKEN
```

#### Deploy a Pod to Consume Metrics

Ensure that the Pod is deployed in a namespace labeled to enforce restricted permissions. Apply the following:

```shell
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: curl-metrics
  namespace: olmv1-system
spec:
  serviceAccountName: operator-controller-controller-manager
  containers:
  - name: curl
    image: curlimages/curl:latest
    command:
    - sh
    - -c
    - sleep 3600
    securityContext:
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      runAsUser: 1000
      runAsGroup: 1000
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
    volumeMounts:
    - mountPath: /tmp/cert
      name: olm-cert
      readOnly: true
  volumes:
  - name: olm-cert
    secret:
      secretName: olmv1-cert
  securityContext:
    runAsNonRoot: true
  restartPolicy: Never
EOF
```

#### Access the Pod and Test Metrics

Access the pod:

```shell
kubectl exec -it curl-metrics -n olmv1-system -- sh
```

From the shell use the `TOKEN` value obtained above to check the metrics:

```shell
curl -v -k -H "Authorization: Bearer <TOKEN>" \
https://operator-controller-service.olmv1-system.svc.cluster.local:8443/metrics
```

Validate using certificates and token:

```shell
curl -v --cacert /tmp/cert/ca.crt --cert /tmp/cert/tls.crt --key /tmp/cert/tls.key \
-H "Authorization: Bearer <TOKEN>" \
https://operator-controller-service.olmv1-system.svc.cluster.local:8443/metrics
```

---

## CatalogD Metrics

### Step 1: Enable Access

To enable access to the CatalogD metrics, create a `ClusterRoleBinding` for the CatalogD service account:

```shell
kubectl create clusterrolebinding catalogd-metrics-binding \
   --clusterrole=catalogd-metrics-reader \
   --serviceaccount=olmv1-system:catalogd-controller-manager
```

### Step 2: Validate Access Manually

#### Create a Token and Extract Certificates

Generate a token and get the required certificates:

```shell
TOKEN=$(kubectl create token catalogd-controller-manager -n olmv1-system)
echo $TOKEN
```

#### Deploy a Pod to Consume Metrics

From the shell use the `TOKEN` value obtained above to check the metrics:

```shell
OLM_SECRET=$(kubectl get secret -n olmv1-system -o jsonpath="{.items[*].metadata.name}" | tr ' ' '\n' | grep '^catalogd-service-cert')
echo $OLM_SECRET
```

```shell
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: curl-metrics-catalogd
  namespace: olmv1-system
spec:
  serviceAccountName: catalogd-controller-manager
  containers:
  - name: curl
    image: curlimages/curl:latest
    command:
    - sh
    - -c
    - sleep 3600
    securityContext:
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      runAsUser: 1000
      runAsGroup: 1000
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
    volumeMounts:
    - mountPath: /tmp/cert
      name: catalogd-cert
      readOnly: true
  volumes:
  - name: catalogd-cert
    secret:
      secretName: $OLM_SECRET
  securityContext:
    runAsNonRoot: true
  restartPolicy: Never
EOF
```

#### Access the Pod and Test Metrics

Access the pod:

```shell
kubectl exec -it curl-metrics-catalogd -n olmv1-system -- sh
```

From the shell use the `TOKEN` value obtained above to check the metrics:

```shell
curl -v -k -H "Authorization: Bearer <TOKEN>" \
https://catalogd-service.olmv1-system.svc.cluster.local:7443/metrics
```

Validate using certificates and token:

```shell
curl -v --cacert /tmp/cert/ca.crt --cert /tmp/cert/tls.crt --key /tmp/cert/tls.key \
-H "Authorization: Bearer <TOKEN>" \
https://catalogd-service.olmv1-system.svc.cluster.local:7443/metrics
```

---

## Enabling Integration with Prometheus

If using [Prometheus Operator][prometheus-operator], create a `ServiceMonitor` to scrape metrics:

!!! note
The following manifests are provided as examples. You may need to configure certain settings, such as `serviceMonitorSelector`,  
to ensure that metrics are properly scraped. This will depend on how Prometheus is configured and, for example, the namespace  
where the `ServiceMonitor` is applied.

### For Operator-Controller

```shell
kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    control-plane: operator-controller-controller-manager
  name: controller-manager-metrics-monitor
  namespace: olmv1-system
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: false 
        serverName: operator-controller-service.olmv1-system.svc
        ca:
          secret:
            name: olmv1-cert
            key: ca.crt
        cert:
          secret:
            name: olmv1-cert
            key: tls.crt
        keySecret:
          name: olmv1-cert
          key: tls.key
  selector:
    matchLabels:
      control-plane: operator-controller-controller-manager
EOF
```

### For CatalogD


```shell
OLM_SECRET=$(kubectl get secret -n olmv1-system -o jsonpath="{.items[*].metadata.name}" | tr ' ' '\n' | grep '^catalogd-service-cert')
echo $OLM_SECRET
```

```shell
kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    control-plane: catalogd-controller-manager
  name: catalogd-metrics-monitor
  namespace: olmv1-system
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        serverName: catalogd-service.olmv1-system.svc
        insecureSkipVerify: false
        ca:
          secret:
            name: $OLM_SECRET
            key: ca.crt
        cert:
          secret:
            name: $OLM_SECRET
            key: tls.crt
        keySecret:
          name: $OLM_SECRET
          key: tls.key
  selector:
    matchLabels:
      control-plane: catalogd-controller-manager
EOF
```

[prometheus-operator]: https://github.com/prometheus-operator/kube-prometheus
[rbac-k8s-docs]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
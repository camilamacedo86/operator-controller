// Package e2e contains end-to-end tests to verify that the metrics endpoints
// for both components. Metrics are exported and accessible by authorized users through
// RBAC and ServiceAccount tokens.
//
// These tests perform the following steps:
// 1. Create a ClusterRoleBinding to grant necessary permissions for accessing metrics.
// 2. Generate a ServiceAccount token for authentication.
// 3. Deploy a curl pod to interact with the metrics endpoint.
// 4. Wait for the curl pod to become ready.
// 5. Execute a curl command from the pod to validate the metrics endpoint.
// 6. Clean up all resources created during the test, such as the ClusterRoleBinding and curl pod.
//
//nolint:gosec
package e2e

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/operator-framework/operator-controller/test/utils"
)

// TestMetricsExportedEndpoint verifies that the metrics endpoint for catalogd and operator-controller
func TestMetricsExportedEndpoint(t *testing.T) {
	client := utils.FindK8sClient(t)

	t.Log("Checking Metrics Endpoint For OperatorController")
	configControllerRuntime := NewMetricsTestConfig(
		t, client,
		"control-plane=operator-controller-controller-manager",
		"operator-controller-metrics-reader",
		"operator-controller-metrics-binding",
		"operator-controller-controller-manager",
		"oper-curl-metrics",
		"https://operator-controller-service.NAMESPACE.svc.cluster.local:8443/metrics",
	)
	configControllerRuntime.run()

	t.Log("Checking Metrics Endpoint For CatalogD")
	configCatalogd := NewMetricsTestConfig(
		t, client,
		"control-plane=catalogd-controller-manager",
		"catalogd-metrics-reader",
		"catalogd-metrics-binding",
		"catalogd-controller-manager",
		"catalogd-curl-metrics",
		"https://catalogd-service.NAMESPACE.svc.cluster.local:7443/metrics",
	)
	configCatalogd.run()

	defer configControllerRuntime.cleanup()
	defer configCatalogd.cleanup()
}

// MetricsTestConfig holds the necessary configurations for testing metrics endpoints.
type MetricsTestConfig struct {
	t              *testing.T
	client         string
	namespace      string
	clusterRole    string
	clusterBinding string
	serviceAccount string
	curlPodName    string
	metricsURL     string
}

// NewMetricsTestConfig initializes a new MetricsTestConfig.
func NewMetricsTestConfig(t *testing.T, client, selector, clusterRole, clusterBinding, serviceAccount, curlPodName, metricsURL string) *MetricsTestConfig {
	namespace := getComponentNamespace(t, client, selector)
	metricsURL = strings.ReplaceAll(metricsURL, "NAMESPACE", namespace)

	return &MetricsTestConfig{
		t:              t,
		client:         client,
		namespace:      namespace,
		clusterRole:    clusterRole,
		clusterBinding: clusterBinding,
		serviceAccount: serviceAccount,
		curlPodName:    curlPodName,
		metricsURL:     metricsURL,
	}
}

// run will execute all steps of those tests
func (c *MetricsTestConfig) run() {
	c.createMetricsClusterRoleBinding()
	token := c.getServiceAccountToken()
	c.createCurlMetricsPod()
	c.validate(token)
}

// createMetricsClusterRoleBinding to binding and expose the metrics
func (c *MetricsTestConfig) createMetricsClusterRoleBinding() {
	c.t.Logf("Creating ClusterRoleBinding %s in namespace %s", c.clusterBinding, c.namespace)
	cmd := exec.Command(c.client, "create", "clusterrolebinding", c.clusterBinding,
		"--clusterrole="+c.clusterRole,
		"--serviceaccount="+c.namespace+":"+c.serviceAccount)
	output, err := cmd.CombinedOutput()
	require.NoError(c.t, err, "Error creating ClusterRoleBinding: %s", string(output))
}

// getServiceAccountToken return the token requires to have access to the metrics
func (c *MetricsTestConfig) getServiceAccountToken() string {
	c.t.Logf("Generating ServiceAccount token at namespace %s", c.namespace)
	cmd := exec.Command(c.client, "create", "token", c.serviceAccount, "-n", c.namespace)
	tokenOutput, tokenCombinedOutput, err := stdoutAndCombined(cmd)
	require.NoError(c.t, err, "Error creating token: %s", string(tokenCombinedOutput))
	return string(bytes.TrimSpace(tokenOutput))
}

// createCurlMetricsPod creates the Pod with curl image to allow check if the metrics are working
func (c *MetricsTestConfig) createCurlMetricsPod() {
	c.t.Logf("Creating curl pod (%s/%s) to validate the metrics endpoint", c.namespace, c.curlPodName)
	cmd := exec.Command(c.client, "run", c.curlPodName,
		"--image=curlimages/curl", "-n", c.namespace,
		"--restart=Never",
		"--overrides", `{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl",
					"command": ["sh", "-c", "sleep 3600"],
					"securityContext": {
						"allowPrivilegeEscalation": false,
						"capabilities": {"drop": ["ALL"]},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {"type": "RuntimeDefault"}
					}
				}],
				"serviceAccountName": "`+c.serviceAccount+`"
			}
		}`)
	output, err := cmd.CombinedOutput()
	require.NoError(c.t, err, "Error creating curl pod: %s", string(output))
}

// validate verifies if is possible to access the metrics
func (c *MetricsTestConfig) validate(token string) {
	c.t.Log("Waiting for the curl pod to be ready")
	waitCmd := exec.Command(c.client, "wait", "--for=condition=Ready", "pod", c.curlPodName, "-n", c.namespace, "--timeout=60s")
	waitOutput, waitErr := waitCmd.CombinedOutput()
	require.NoError(c.t, waitErr, "Error waiting for curl pod to be ready: %s", string(waitOutput))

	c.t.Log("Validating the metrics endpoint")
	curlCmd := exec.Command(c.client, "exec", c.curlPodName, "-n", c.namespace, "--",
		"curl", "-v", "-k", "-H", "Authorization: Bearer "+token, c.metricsURL)
	output, err := curlCmd.CombinedOutput()
	require.NoError(c.t, err, "Error calling metrics endpoint: %s", string(output))
	require.Contains(c.t, string(output), "200 OK", "Metrics endpoint did not return 200 OK")
}

// cleanup created resources
func (c *MetricsTestConfig) cleanup() {
	c.t.Log("Cleaning up resources")
	_ = exec.Command(c.client, "delete", "clusterrolebinding", c.clusterBinding, "--ignore-not-found=true").Run()
	_ = exec.Command(c.client, "delete", "pod", c.curlPodName, "-n", c.namespace, "--ignore-not-found=true").Run()
}

// getComponentNamespace returns the namespace where operator-controller or catalogd is running
func getComponentNamespace(t *testing.T, client, selector string) string {
	cmd := exec.Command(client, "get", "pods", "--all-namespaces", "--selector="+selector, "--output=jsonpath={.items[0].metadata.namespace}")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Error determining namespace: %s", string(output))

	namespace := string(bytes.TrimSpace(output))
	if namespace == "" {
		t.Fatal("No namespace found for selector " + selector)
	}
	return namespace
}

func stdoutAndCombined(cmd *exec.Cmd) ([]byte, []byte, error) {
	var outOnly, outAndErr bytes.Buffer
	allWriter := io.MultiWriter(&outOnly, &outAndErr)
	cmd.Stdout = allWriter
	cmd.Stderr = &outAndErr
	err := cmd.Run()
	return outOnly.Bytes(), outAndErr.Bytes(), err
}

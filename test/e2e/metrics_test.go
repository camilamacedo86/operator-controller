package e2e

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// nolint:gosec
// TestOperatorControllerMetricsExportedEndpoint verifies that the metrics endpoint for the operator controller
// is exported correctly and accessible by authorized users through RBAC and a ServiceAccount token.
// The test performs the following steps:
// 1. Creates a ClusterRoleBinding to grant necessary permissions for accessing metrics.
// 2. Generates a ServiceAccount token for authentication.
// 3. Deploys a curl pod to interact with the metrics endpoint.
// 4. Waits for the curl pod to become ready.
// 5. Executes a curl command from the pod to validate the metrics endpoint.
// 6. Cleans up all resources created during the test, such as the ClusterRoleBinding and curl pod.
func TestOperatorControllerMetricsExportedEndpoint(t *testing.T) {
	var (
		token     string
		curlPod   = "curl-metrics"
		namespace = "olmv1-system"
	)

	t.Log("Creating ClusterRoleBinding for operator controller metrics")
	cmd := exec.Command("kubectl", "create", "clusterrolebinding", "operator-controller-metrics-binding",
		"--clusterrole=operator-controller-metrics-reader",
		"--serviceaccount="+namespace+":operator-controller-controller-manager")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Error creating ClusterRoleBinding: %s", string(output))

	defer func() {
		t.Log("Cleaning up ClusterRoleBinding")
		_ = exec.Command("kubectl", "delete", "clusterrolebinding", "operator-controller-metrics-binding", "--ignore-not-found=true").Run()
	}()

	t.Log("Generating ServiceAccount token")
	tokenCmd := exec.Command("kubectl", "create", "token", "operator-controller-controller-manager", "-n", namespace)
	tokenOutput, err := tokenCmd.Output()
	require.NoError(t, err, "Error creating token: %s", string(tokenOutput))
	token = string(bytes.TrimSpace(tokenOutput))

	t.Log("Creating curl pod to validate the metrics endpoint")
	cmd = exec.Command("kubectl", "run", curlPod,
		"--image=curlimages/curl:7.87.0", "-n", namespace,
		"--restart=Never",
		"--overrides", `{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl:7.87.0",
					"command": ["sh", "-c", "sleep 3600"],
					"securityContext": {
						"allowPrivilegeEscalation": false,
						"capabilities": {
							"drop": ["ALL"]
						},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {
							"type": "RuntimeDefault"
						}
					}
				}],
				"serviceAccountName": "operator-controller-controller-manager"
			}
		}`)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "Error creating curl pod: %s", string(output))

	defer func() {
		t.Log("Cleaning up curl pod")
		_ = exec.Command("kubectl", "delete", "pod", curlPod, "-n", namespace, "--ignore-not-found=true").Run()
	}()

	t.Log("Waiting for the curl pod to be ready")
	waitCmd := exec.Command("kubectl", "wait", "--for=condition=Ready", "pod", curlPod, "-n", namespace, "--timeout=60s")
	waitOutput, waitErr := waitCmd.CombinedOutput()
	require.NoError(t, waitErr, "Error waiting for curl pod to be ready: %s", string(waitOutput))

	t.Log("Validating the metrics endpoint")
	metricsURL := "https://operator-controller-controller-manager-metrics-service." + namespace + ".svc.cluster.local:8443/metrics"
	curlCmd := exec.Command("kubectl", "exec", curlPod, "-n", namespace, "--",
		"curl", "-v", "-k", "-H", "Authorization: Bearer "+token, metricsURL)
	output, err = curlCmd.CombinedOutput()
	require.NoError(t, err, "Error calling metrics endpoint: %s", string(output))
	require.Contains(t, string(output), "200 OK", "Metrics endpoint did not return 200 OK")
}

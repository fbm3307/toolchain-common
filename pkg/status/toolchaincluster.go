package status

import (
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"

	errs "github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// error messages related to cluster connection
const (
	ErrMsgClusterConnectionNotFound              = "the cluster connection was not found"
	ErrMsgClusterConnectionLastProbeTimeExceeded = "exceeded the maximum duration since the last probe"
)

// ToolchainClusterAttributes required attributes for obtaining ToolchainCluster status
type ToolchainClusterAttributes struct {
	GetClusterFunc func() (*cluster.CachedToolchainCluster, bool)
	Period         time.Duration
	Timeout        time.Duration
}

// GetToolchainClusterConditions uses the provided ToolchainCluster attributes to determine status conditions
func GetToolchainClusterConditions(attrs ToolchainClusterAttributes) []toolchainv1alpha1.Condition {
	// look up cluster connection status
	toolchainCluster, ok := attrs.GetClusterFunc()
	if !ok {
		return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionNotFoundReason, ErrMsgClusterConnectionNotFound)}
	}

	// check conditions of cluster connection
	if !cluster.IsReady(toolchainCluster.ClusterStatus) {
		for _, c := range toolchainCluster.ClusterStatus.Conditions {
			if c.Type == "Ready" && c.Message != "" {
				return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionNotReadyReason, c.Message)}
			}
		}
		genericErrMsg := "the cluster connection is not ready"
		return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionNotReadyReason, genericErrMsg)}
	}

	var lastProbeTime metav1.Time
	foundLastProbeTime := false
	for _, condition := range toolchainCluster.ClusterStatus.Conditions {
		if condition.Type == toolchainv1alpha1.ToolchainClusterReady {
			lastProbeTime = condition.LastProbeTime
			foundLastProbeTime = true
		}
	}
	if !foundLastProbeTime {
		lastProbeNotFoundMsg := "the time of the last probe could not be determined"
		return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionNotReadyReason, lastProbeNotFoundMsg)}
	}

	// check that the last probe time is within limits. It should be less than (period + timeout) * threshold
	totalf := attrs.Period.Seconds() + attrs.Timeout.Seconds()
	maxDuration, err := time.ParseDuration(fmt.Sprintf("%fs", totalf))
	if err != nil {
		invalidLastProbeMsg := "the maximum duration since the last probe could not be determined - check the configured values for the ToolchainCluster health check period, timeout and failure threshold"
		wrappedErr := errs.Wrap(err, invalidLastProbeMsg)
		return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionNotReadyReason, wrappedErr.Error())}
	}

	lastProbedTimePlusMaxDuration := lastProbeTime.Add(maxDuration)
	currentTime := time.Now()
	if currentTime.After(lastProbedTimePlusMaxDuration) {
		errMsg := fmt.Sprintf("%s: %s", ErrMsgClusterConnectionLastProbeTimeExceeded, maxDuration.String())
		return []toolchainv1alpha1.Condition{*NewComponentErrorCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionLastProbeTimeExceededReason, errMsg)}
	}
	return []toolchainv1alpha1.Condition{*NewComponentReadyCondition(toolchainv1alpha1.ToolchainStatusClusterConnectionReadyReason)}
}
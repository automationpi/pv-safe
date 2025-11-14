package webhook

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RiskAssessment contains the result of analyzing deletion risk
type RiskAssessment struct {
	IsRisky      bool
	RiskyPVCs    []RiskyPVC
	Message      string
	Suggestion   string
}

// RiskyPVC represents a PVC that would lose data if deleted
type RiskyPVC struct {
	Name           string
	Namespace      string
	PVName         string
	Reason         string
	HasSnapshot    bool
	SnapshotInfo   string
}

// RiskCalculator analyzes deletion risk for PVs and PVCs
type RiskCalculator struct {
	client          kubernetes.Interface
	snapshotChecker *SnapshotChecker
}

// NewRiskCalculator creates a new risk calculator
func NewRiskCalculator(client kubernetes.Interface, snapshotChecker *SnapshotChecker) *RiskCalculator {
	return &RiskCalculator{
		client:          client,
		snapshotChecker: snapshotChecker,
	}
}

// AssessNamespaceDeletion checks if deleting a namespace would lose data
func (rc *RiskCalculator) AssessNamespaceDeletion(ctx context.Context, namespace string) (*RiskAssessment, error) {
	pvcs, err := rc.client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs in namespace %s: %w", namespace, err)
	}

	if len(pvcs.Items) == 0 {
		return &RiskAssessment{
			IsRisky: false,
			Message: fmt.Sprintf("Namespace %s has no PVCs", namespace),
		}, nil
	}

	assessment := &RiskAssessment{
		IsRisky:   false,
		RiskyPVCs: []RiskyPVC{},
	}

	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}

		pv, err := rc.client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			continue
		}

		isRisky, reason, snapshotInfo := rc.isPVCRisky(ctx, pvc.Namespace, pvc.Name, pv)
		if isRisky {
			assessment.IsRisky = true
			riskyPVC := RiskyPVC{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
				PVName:    pv.Name,
				Reason:    reason,
			}
			if snapshotInfo != nil {
				riskyPVC.HasSnapshot = true
				riskyPVC.SnapshotInfo = snapshotInfo.Name
			}
			assessment.RiskyPVCs = append(assessment.RiskyPVCs, riskyPVC)
		}
	}

	if assessment.IsRisky {
		assessment.Message = rc.buildNamespaceBlockMessage(namespace, assessment.RiskyPVCs)
		assessment.Suggestion = rc.buildSuggestions(namespace, assessment.RiskyPVCs)
	}

	return assessment, nil
}

// AssessPVCDeletion checks if deleting a PVC would lose data
func (rc *RiskCalculator) AssessPVCDeletion(ctx context.Context, namespace, name string) (*RiskAssessment, error) {
	pvc, err := rc.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PVC %s/%s: %w", namespace, name, err)
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return &RiskAssessment{
			IsRisky: false,
			Message: fmt.Sprintf("PVC %s/%s is not bound to a PV", namespace, name),
		}, nil
	}

	pv, err := rc.client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PV %s: %w", pvc.Spec.VolumeName, err)
	}

	isRisky, reason, snapshotInfo := rc.isPVCRisky(ctx, namespace, name, pv)

	assessment := &RiskAssessment{
		IsRisky: isRisky,
	}

	if assessment.IsRisky {
		riskyPVC := RiskyPVC{
			Name:      name,
			Namespace: namespace,
			PVName:    pv.Name,
			Reason:    reason,
		}
		if snapshotInfo != nil {
			riskyPVC.HasSnapshot = true
			riskyPVC.SnapshotInfo = snapshotInfo.Name
		}
		assessment.RiskyPVCs = []RiskyPVC{riskyPVC}
		assessment.Message = rc.buildPVCBlockMessage(riskyPVC)
		assessment.Suggestion = rc.buildPVCSuggestions(namespace, name, pv.Name)
	} else if snapshotInfo != nil {
		// Not risky because snapshot exists - include this info in the message
		assessment.Message = reason
	}

	return assessment, nil
}

// AssessPVDeletion checks if deleting a PV would lose data
func (rc *RiskCalculator) AssessPVDeletion(ctx context.Context, pvName string) (*RiskAssessment, error) {
	pv, err := rc.client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PV %s: %w", pvName, err)
	}

	assessment := &RiskAssessment{
		IsRisky: rc.isPVRisky(pv),
	}

	if assessment.IsRisky {
		namespace := ""
		pvcName := ""
		if pv.Spec.ClaimRef != nil {
			namespace = pv.Spec.ClaimRef.Namespace
			pvcName = pv.Spec.ClaimRef.Name
		}

		riskyPVC := RiskyPVC{
			Name:      pvcName,
			Namespace: namespace,
			PVName:    pv.Name,
			Reason:    fmt.Sprintf("PV has %s reclaim policy, no snapshot found", pv.Spec.PersistentVolumeReclaimPolicy),
		}
		assessment.RiskyPVCs = []RiskyPVC{riskyPVC}
		assessment.Message = rc.buildPVBlockMessage(pv, riskyPVC)
		assessment.Suggestion = rc.buildPVSuggestions(pv)
	}

	return assessment, nil
}

// isPVRisky determines if a PV deletion would cause data loss
func (rc *RiskCalculator) isPVRisky(pv *corev1.PersistentVolume) bool {
	// Safe if reclaim policy is Retain
	if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
		return false
	}

	// Risky if reclaim policy is Delete
	if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
		return true
	}

	// Default to risky for unknown policies
	return true
}

// isPVCRisky determines if a PVC deletion would cause data loss, considering snapshots
func (rc *RiskCalculator) isPVCRisky(ctx context.Context, namespace, pvcName string, pv *corev1.PersistentVolume) (bool, string, *SnapshotInfo) {
	// Safe if reclaim policy is Retain
	if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
		return false, "PV has Retain reclaim policy", nil
	}

	// If reclaim policy is Delete, check for snapshots
	if rc.snapshotChecker != nil {
		hasSnapshot, snapshotInfo, err := rc.snapshotChecker.HasReadySnapshot(ctx, namespace, pvcName)
		if err == nil && hasSnapshot && snapshotInfo != nil {
			// Safe if there's a ready snapshot with Retain policy
			return false, fmt.Sprintf("Ready VolumeSnapshot '%s' exists with Retain policy", snapshotInfo.Name), snapshotInfo
		}
	}

	// Risky: Delete reclaim policy and no snapshot
	return true, fmt.Sprintf("PV has %s reclaim policy, no snapshot found", pv.Spec.PersistentVolumeReclaimPolicy), nil
}

// buildNamespaceBlockMessage creates a user-friendly error message for namespace deletion
func (rc *RiskCalculator) buildNamespaceBlockMessage(namespace string, riskyPVCs []RiskyPVC) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("DELETION BLOCKED: Namespace '%s' contains %d PVC(s) that would lose data permanently\n\n", namespace, len(riskyPVCs)))
	sb.WriteString("Risky PVCs:\n")

	for _, risky := range riskyPVCs {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", risky.Name, risky.Reason))
	}

	return sb.String()
}

// buildPVCBlockMessage creates a user-friendly error message for PVC deletion
func (rc *RiskCalculator) buildPVCBlockMessage(risky RiskyPVC) string {
	return fmt.Sprintf("DELETION BLOCKED: PVC '%s/%s' would lose data permanently\n\nReason: %s\n",
		risky.Namespace, risky.Name, risky.Reason)
}

// buildPVBlockMessage creates a user-friendly error message for PV deletion
func (rc *RiskCalculator) buildPVBlockMessage(pv *corev1.PersistentVolume, risky RiskyPVC) string {
	msg := fmt.Sprintf("DELETION BLOCKED: PV '%s' would lose data permanently\n\nReason: %s\n",
		pv.Name, risky.Reason)

	if risky.Namespace != "" && risky.Name != "" {
		msg += fmt.Sprintf("Bound to: %s/%s\n", risky.Namespace, risky.Name)
	}

	return msg
}

// buildPVCSuggestions creates actionable suggestions for PVC deletion
func (rc *RiskCalculator) buildPVCSuggestions(namespace, pvcName, pvName string) string {
	return fmt.Sprintf("\nTo safely delete this PVC:\n"+
		"  1. Create a VolumeSnapshot of the data\n"+
		"  2. OR change PV reclaim policy to Retain:\n"+
		"     kubectl patch pv %s -p '{\"spec\":{\"persistentVolumeReclaimPolicy\":\"Retain\"}}'\n"+
		"\n  3. OR force delete (will lose data):\n"+
		"     kubectl label pvc %s -n %s pv-safe.io/force-delete=true\n"+
		"     kubectl delete pvc %s -n %s\n"+
		"\n  4. Then retry the deletion\n", pvName, pvcName, namespace, pvcName, namespace)
}

// buildSuggestions creates actionable suggestions for safe deletion
func (rc *RiskCalculator) buildSuggestions(namespace string, riskyPVCs []RiskyPVC) string {
	var sb strings.Builder

	sb.WriteString("\nTo safely delete this resource:\n")
	sb.WriteString("  1. Create VolumeSnapshots for the PVCs\n")
	sb.WriteString("  2. OR change PV reclaim policy to Retain:\n")

	for _, risky := range riskyPVCs {
		sb.WriteString(fmt.Sprintf("     kubectl patch pv %s -p '{\"spec\":{\"persistentVolumeReclaimPolicy\":\"Retain\"}}'\n", risky.PVName))
	}

	sb.WriteString(fmt.Sprintf("\n  3. OR force delete (will lose data):\n"))
	sb.WriteString(fmt.Sprintf("     kubectl label namespace %s pv-safe.io/force-delete=true\n", namespace))
	sb.WriteString(fmt.Sprintf("     kubectl delete namespace %s\n", namespace))

	sb.WriteString("\n  4. Then retry the deletion\n")

	return sb.String()
}

// buildPVSuggestions creates actionable suggestions for PV deletion
func (rc *RiskCalculator) buildPVSuggestions(pv *corev1.PersistentVolume) string {
	return fmt.Sprintf("\nTo safely delete this PV:\n"+
		"  1. Create a VolumeSnapshot of the data\n"+
		"  2. OR change reclaim policy to Retain:\n"+
		"     kubectl patch pv %s -p '{\"spec\":{\"persistentVolumeReclaimPolicy\":\"Retain\"}}'\n"+
		"\n  3. OR force delete (will lose data):\n"+
		"     kubectl label pv %s pv-safe.io/force-delete=true\n"+
		"     kubectl delete pv %s\n"+
		"\n  4. Then retry the deletion\n", pv.Name, pv.Name, pv.Name)
}

package webhook

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// UnknownDeletionPolicy represents an unknown or unset deletion policy
	UnknownDeletionPolicy = "Unknown"
)

// SnapshotChecker checks for VolumeSnapshots
type SnapshotChecker struct {
	dynamicClient dynamic.Interface
	clientset     kubernetes.Interface
}

// NewSnapshotChecker creates a new snapshot checker
func NewSnapshotChecker(config *rest.Config, clientset kubernetes.Interface) (*SnapshotChecker, error) {
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &SnapshotChecker{
		dynamicClient: dynamicClient,
		clientset:     clientset,
	}, nil
}

var volumeSnapshotGVR = schema.GroupVersionResource{
	Group:    "snapshot.storage.k8s.io",
	Version:  "v1",
	Resource: "volumesnapshots",
}

// SnapshotInfo contains information about a VolumeSnapshot
type SnapshotInfo struct {
	Name           string
	Namespace      string
	SourcePVC      string
	IsReady        bool
	DeletionPolicy string
	CreationTime   metav1.Time
	RestoreSize    string
}

// HasReadySnapshot checks if a PVC has a Ready VolumeSnapshot with Retain policy
func (sc *SnapshotChecker) HasReadySnapshot(ctx context.Context, namespace, pvcName string) (bool, *SnapshotInfo, error) {
	snapshots, err := sc.dynamicClient.Resource(volumeSnapshotGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		// VolumeSnapshot CRD might not be installed
		return false, nil, fmt.Errorf("failed to list volumesnapshots (CSI snapshots may not be available): %w", err)
	}

	for _, item := range snapshots.Items {
		snapshot := item.Object

		// Check if this snapshot is for our PVC
		sourcePVC, found, err := unstructured.NestedString(snapshot, "spec", "source", "persistentVolumeClaimName")
		if err != nil || !found || sourcePVC != pvcName {
			continue
		}

		// Check if snapshot is ready
		ready, _, _ := unstructured.NestedBool(snapshot, "status", "readyToUse")
		if !ready {
			continue
		}

		// Get deletion policy from VolumeSnapshotClass if possible
		deletionPolicy := UnknownDeletionPolicy
		snapshotClassName, found, _ := unstructured.NestedString(snapshot, "spec", "volumeSnapshotClassName")
		if found && snapshotClassName != "" {
			policy, err := sc.getSnapshotClassDeletionPolicy(ctx, snapshotClassName)
			if err == nil {
				deletionPolicy = policy
			}
		}

		// Extract snapshot info
		info := &SnapshotInfo{
			Name:           item.GetName(),
			Namespace:      item.GetNamespace(),
			SourcePVC:      sourcePVC,
			IsReady:        true,
			DeletionPolicy: deletionPolicy,
			CreationTime:   item.GetCreationTimestamp(),
		}

		// Get restore size if available
		if restoreSize, found, _ := unstructured.NestedString(snapshot, "status", "restoreSize"); found {
			info.RestoreSize = restoreSize
		}

		// If snapshot is ready with Retain policy, return true
		if deletionPolicy == "Retain" {
			return true, info, nil
		}

		// If we found a ready snapshot but policy is not Retain, continue looking
		// There might be another snapshot with Retain policy
	}

	return false, nil, nil
}

// getSnapshotClassDeletionPolicy gets the deletion policy from a VolumeSnapshotClass
func (sc *SnapshotChecker) getSnapshotClassDeletionPolicy(ctx context.Context, className string) (string, error) {
	snapshotClassGVR := schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshotclasses",
	}

	class, err := sc.dynamicClient.Resource(snapshotClassGVR).Get(ctx, className, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	policy, found, err := unstructured.NestedString(class.Object, "deletionPolicy")
	if err != nil || !found {
		return UnknownDeletionPolicy, nil
	}

	return policy, nil
}

// ListSnapshots lists all snapshots for a PVC
func (sc *SnapshotChecker) ListSnapshots(ctx context.Context, namespace, pvcName string) ([]*SnapshotInfo, error) {
	snapshots, err := sc.dynamicClient.Resource(volumeSnapshotGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumesnapshots: %w", err)
	}

	var result []*SnapshotInfo

	for _, item := range snapshots.Items {
		snapshot := item.Object

		// Check if this snapshot is for our PVC
		sourcePVC, found, err := unstructured.NestedString(snapshot, "spec", "source", "persistentVolumeClaimName")
		if err != nil || !found || sourcePVC != pvcName {
			continue
		}

		ready, _, _ := unstructured.NestedBool(snapshot, "status", "readyToUse")

		deletionPolicy := UnknownDeletionPolicy
		snapshotClassName, found, _ := unstructured.NestedString(snapshot, "spec", "volumeSnapshotClassName")
		if found && snapshotClassName != "" {
			policy, err := sc.getSnapshotClassDeletionPolicy(ctx, snapshotClassName)
			if err == nil {
				deletionPolicy = policy
			}
		}

		info := &SnapshotInfo{
			Name:           item.GetName(),
			Namespace:      item.GetNamespace(),
			SourcePVC:      sourcePVC,
			IsReady:        ready,
			DeletionPolicy: deletionPolicy,
			CreationTime:   item.GetCreationTimestamp(),
		}

		if restoreSize, found, _ := unstructured.NestedString(snapshot, "status", "restoreSize"); found {
			info.RestoreSize = restoreSize
		}

		result = append(result, info)
	}

	return result, nil
}

// IsSnapshotAPIAvailable checks if the VolumeSnapshot CRD is installed
func (sc *SnapshotChecker) IsSnapshotAPIAvailable(ctx context.Context) bool {
	_, err := sc.dynamicClient.Resource(volumeSnapshotGVR).List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil
}

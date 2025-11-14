// Package webhook implements a Kubernetes admission webhook handler.
// This webhook intercepts admission requests (CREATE, UPDATE, DELETE operations)
// and logs them for auditing purposes, with special attention to deletion
// operations on critical resources like PersistentVolumes, PersistentVolumeClaims, and Namespaces.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
)

const (
	// BypassLabel is the label that allows forcing deletion
	BypassLabel = "pv-safe.io/force-delete"
)

// Handler is the main webhook handler that processes Kubernetes admission requests.
// It contains a logger for structured logging and a risk calculator for assessing deletions.
type Handler struct {
	Logger         *log.Logger
	RiskCalculator *RiskCalculator
}

// NewHandler creates a new webhook handler instance with the provided logger, client, and snapshot checker.
// This is the constructor function for the Handler struct.
func NewHandler(logger *log.Logger, client kubernetes.Interface, snapshotChecker *SnapshotChecker) *Handler {
	return &Handler{
		Logger:         logger,
		RiskCalculator: NewRiskCalculator(client, snapshotChecker),
	}
}

// ServeHTTP is the main HTTP handler that implements the http.Handler interface.
// This function is called by Kubernetes API server when an admission request is made.
// It processes the incoming AdmissionReview request and returns an AdmissionReview response.
//
// Flow:
// 1. Validates that the request method is POST (required for admission webhooks)
// 2. Reads the request body containing the AdmissionReview JSON
// 3. Parses the JSON into an AdmissionReview struct
// 4. Validates that the request field is present
// 5. Processes the admission request and generates a response
// 6. Marshals the response back to JSON and sends it to Kubernetes
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Logger.Printf("Received request: %s %s", r.Method, r.URL.Path)

	// Admission webhooks must use POST method - reject all other methods
	if r.Method != http.MethodPost {
		h.Logger.Printf("Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the entire request body which contains the AdmissionReview JSON
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.Logger.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	h.Logger.Printf("Request body size: %d bytes", len(body))

	// Parse the JSON body into a Kubernetes AdmissionReview struct
	var admissionReview admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		h.Logger.Printf("Error unmarshaling admission review: %v", err)
		http.Error(w, "Error parsing admission review", http.StatusBadRequest)
		return
	}

	// Ensure the request field is present (required by Kubernetes API)
	if admissionReview.Request == nil {
		h.Logger.Printf("Admission review request is nil")
		http.Error(w, "Invalid admission review", http.StatusBadRequest)
		return
	}

	// Process the admission request and generate a response
	response := h.handleAdmissionRequest(admissionReview.Request)

	// Build the response AdmissionReview object
	// Kubernetes requires the response to be wrapped in an AdmissionReview with proper TypeMeta
	admissionResponse := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: response,
	}

	// Convert the response to JSON
	responseBytes, err := json.Marshal(admissionResponse)
	if err != nil {
		h.Logger.Printf("Error marshaling response: %v", err)
		http.Error(w, "Error creating response", http.StatusInternalServerError)
		return
	}

	// Set the content type header and send the JSON response back to Kubernetes
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(responseBytes); err != nil {
		h.Logger.Printf("Error writing response: %v", err)
	}
}

// handleAdmissionRequest processes an individual admission request and generates a response.
// It logs all the important details about the request and handles special cases
// (like DELETE operations) with risk assessment and potential blocking.
//
// Parameters:
//   - request: The Kubernetes AdmissionRequest containing details about the operation
//
// Returns:
//   - An AdmissionResponse that either allows or denies the request based on risk assessment
func (h *Handler) handleAdmissionRequest(request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Log comprehensive details about the admission request for auditing
	h.Logger.Printf("========================================")
	h.Logger.Printf("Admission Request Details:")
	h.Logger.Printf("  UID: %s", request.UID)
	h.Logger.Printf("  Operation: %s", request.Operation)
	h.Logger.Printf("  Kind: %s", request.Kind.Kind)
	h.Logger.Printf("  Namespace: %s", request.Namespace)
	h.Logger.Printf("  Name: %s", request.Name)
	h.Logger.Printf("  User: %s", request.UserInfo.Username)
	h.Logger.Printf("  Groups: %v", request.UserInfo.Groups)
	h.Logger.Printf("========================================")

	// Special handling for DELETE operations - assess risk and potentially block
	if request.Operation == admissionv1.Delete {
		h.logDeletion(request)
		return h.assessAndDecide(request)
	}

	// Non-DELETE operations are always allowed
	return &admissionv1.AdmissionResponse{
		UID:     request.UID,
		Allowed: true,
		Result: &metav1.Status{
			Message: "Request allowed",
		},
	}
}

// assessAndDecide performs risk assessment for DELETE operations and decides whether to allow or block
func (h *Handler) assessAndDecide(request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kind := request.Kind.Kind
	namespace := request.Namespace
	name := request.Name

	// Check for bypass label
	if h.hasBypassLabel(request) {
		h.Logger.Printf("BYPASS: Force delete label found on %s %s/%s", kind, namespace, name)
		h.Logger.Printf("  User: %s", request.UserInfo.Username)
		h.Logger.Printf("  Allowing deletion despite potential data loss")
		return &admissionv1.AdmissionResponse{
			UID:     request.UID,
			Allowed: true,
			Result: &metav1.Status{
				Message: fmt.Sprintf("Deletion allowed via bypass label %s", BypassLabel),
			},
		}
	}

	h.Logger.Printf("Assessing risk for %s deletion: %s/%s", kind, namespace, name)

	var assessment *RiskAssessment
	var err error

	switch kind {
	case "Namespace":
		assessment, err = h.RiskCalculator.AssessNamespaceDeletion(ctx, name)
	case "PersistentVolumeClaim":
		assessment, err = h.RiskCalculator.AssessPVCDeletion(ctx, namespace, name)
	case "PersistentVolume":
		assessment, err = h.RiskCalculator.AssessPVDeletion(ctx, name)
	default:
		// Unknown resource type - allow by default
		h.Logger.Printf("Unknown resource type %s - allowing", kind)
		return &admissionv1.AdmissionResponse{
			UID:     request.UID,
			Allowed: true,
		}
	}

	if err != nil {
		h.Logger.Printf("ERROR: Risk assessment failed: %v", err)
		// On error, allow the request (fail open for now)
		return &admissionv1.AdmissionResponse{
			UID:     request.UID,
			Allowed: true,
			Result: &metav1.Status{
				Message: fmt.Sprintf("Risk assessment error (allowed): %v", err),
			},
		}
	}

	if assessment.IsRisky {
		h.Logger.Printf("BLOCKING: Risky deletion detected!")
		h.Logger.Printf("  Reason: %s", assessment.Message)
		h.Logger.Printf("  Risky PVCs: %d", len(assessment.RiskyPVCs))

		message := assessment.Message + assessment.Suggestion

		return &admissionv1.AdmissionResponse{
			UID:     request.UID,
			Allowed: false,
			Result: &metav1.Status{
				Status:  "Failure",
				Message: message,
				Reason:  metav1.StatusReasonForbidden,
				Code:    403,
			},
		}
	}

	h.Logger.Printf("ALLOWING: Deletion is safe")
	if assessment.Message != "" {
		h.Logger.Printf("  Reason: %s", assessment.Message)
	}

	return &admissionv1.AdmissionResponse{
		UID:     request.UID,
		Allowed: true,
		Result: &metav1.Status{
			Message: "Deletion allowed - safe operation",
		},
	}
}

// hasBypassLabel checks if the resource being deleted has the bypass label
func (h *Handler) hasBypassLabel(request *admissionv1.AdmissionRequest) bool {
	// For DELETE operations, the resource being deleted is in OldObject
	if request.OldObject.Raw == nil {
		return false
	}

	// Parse the OldObject to extract labels
	var obj unstructured.Unstructured
	if err := json.Unmarshal(request.OldObject.Raw, &obj); err != nil {
		h.Logger.Printf("Warning: Failed to parse OldObject for bypass check: %v", err)
		return false
	}

	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	value, exists := labels[BypassLabel]
	return exists && value == "true"
}

// logDeletion provides specialized logging for DELETE operations on critical resources.
// This function is called when a deletion is detected and logs detailed information
// about who is attempting to delete what resource.
//
// It has special handling for three critical resource types:
//   - Namespace: Deleting a namespace deletes all resources within it
//   - PersistentVolumeClaim (PVC): Deleting a PVC can cause data loss
//   - PersistentVolume (PV): Deleting a PV can cause permanent data loss
//
// Parameters:
//   - request: The admission request containing deletion details
func (h *Handler) logDeletion(request *admissionv1.AdmissionRequest) {
	// Extract key information from the request
	kind := request.Kind.Kind      // Type of resource being deleted
	namespace := request.Namespace // Namespace (empty for cluster-scoped resources)
	name := request.Name           // Name of the resource
	user := request.UserInfo.Username // User attempting the deletion

	// Provide detailed, resource-specific logging for critical resources
	switch kind {
	case "Namespace":
		// Namespace deletion is very dangerous - it deletes everything in the namespace
		h.Logger.Printf("DELETE NAMESPACE detected!")
		h.Logger.Printf("  Namespace: %s", name)
		h.Logger.Printf("  User: %s", user)
		h.Logger.Printf("  Action: Deletion of namespace '%s' is being attempted", name)

	case "PersistentVolumeClaim":
		// PVC deletion can cause data loss if the reclaim policy allows it
		h.Logger.Printf("DELETE PVC detected!")
		h.Logger.Printf("  PVC: %s/%s", namespace, name) // Format: namespace/name
		h.Logger.Printf("  User: %s", user)
		h.Logger.Printf("  Action: Deletion of PVC '%s' in namespace '%s' is being attempted", name, namespace)

	case "PersistentVolume":
		// PV deletion can cause permanent data loss
		// Note: PVs are cluster-scoped, so namespace is empty
		h.Logger.Printf("DELETE PV detected!")
		h.Logger.Printf("  PV: %s", name)
		h.Logger.Printf("  User: %s", user)
		h.Logger.Printf("  Action: Deletion of PV '%s' is being attempted", name)

	default:
		// Generic logging for other resource types being deleted
		h.Logger.Printf("DELETE %s detected: %s/%s by %s", kind, namespace, name, user)
	}
}

// HealthCheck is a simple health check endpoint that can be used by Kubernetes
// liveness and readiness probes to verify the webhook service is running.
// Returns HTTP 200 with "OK" message when the service is healthy.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

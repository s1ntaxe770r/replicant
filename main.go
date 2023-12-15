package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"

	"golang.org/x/exp/slog"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	port    int
	tlsKey  string
	tlsCert string
)

type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value int32  `json:"value"`
}

func main() {
	flag.IntVar(&port, "port", 9093, "Admisson controller port")
	flag.StringVar(&tlsKey, "tls-key", "/etc/webhook/certs/tls.key", "Private key for TLS")
	flag.StringVar(&tlsCert, "tls-crt", "/etc/webhook/certs/tls.crt", "TLS certificate")
	flag.Parse()

}

func mutate(w http.ResponseWriter, r *http.Request) {
	slog.Info("new mutate request")

	scheme := runtime.NewScheme()
	codecFactory := serializer.NewCodecFactory(scheme)
	deserializer := codecFactory.UniversalDeserializer()

	admissionReviewRequest, err := parseAdmissionReview(r, deserializer)
	if err != nil {
		httpError(w, err)
		return
	}

	deploymentGVR := metav1.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	if admissionReviewRequest.Request.Resource != deploymentGVR {
		err := errors.New("admission request is not of kind: Deployment")
		httpError(w, err)
		return
	}

	deployment := appsv1.Deployment{}

	_, _, err = deserializer.Decode(admissionReviewRequest.Request.Object.Raw, nil, &deployment)
	if err != nil {
		err := errors.New("unable to unmarshall request to deployment")
		httpError(w, err)
		return
	}

	patch := PatchOperation{
		Op:    "replace",
		Path:  "/spec/replicas",
		Value: 3,
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		err := errors.New("unable to marshal patch into bytes")
		httpError(w, err)
		return
	}

	admissionResponse := &admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     admissionReviewRequest.Request.UID,
			Allowed: true,
		},
	}

	admissionResponse.Response.Patch = patchBytes

	responseBytes, err := json.Marshal(&admissionResponse)
	if err != nil {
		err := errors.New("unable to marshal patch response  into bytes")
		httpError(w, err)
		return
	}
	w.Write(responseBytes)
}

func httpError(w http.ResponseWriter, err error) {
	slog.Error("unable to complete request", err.Error())
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(err.Error()))
}
func parseAdmissionReview(req *http.Request, deserializer runtime.Decoder) (*admissionv1.AdmissionReview, error) {

	reqData, err := io.ReadAll(req.Body)
	if err != nil {
		slog.Error("error reading request body", err)
		return nil, err
	}

	admissionReviewRequest := &admissionv1.AdmissionReview{}

	_, _, err = deserializer.Decode(reqData, nil, admissionReviewRequest)
	if err != nil {
		slog.Error("unable to desdeserialize request", err)
		return nil, err
	}

	return admissionReviewRequest, nil
}

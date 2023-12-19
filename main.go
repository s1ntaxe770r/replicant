package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
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
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func main() {
	flag.IntVar(&port, "port", 9093, "Admisson controller port")
	flag.StringVar(&tlsKey, "tls-key", "/etc/webhook/certs/tls.key", "Private key for TLS")
	flag.StringVar(&tlsCert, "tls-crt", "/etc/webhook/certs/tls.crt", "TLS certificate")
	flag.Parse()
	slog.Info("loading certs..")
	certs, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
	if err != nil {
		slog.Error("unable to load certs", err)
	}

	http.HandleFunc("/mutate", mutate)

	slog.Info("successfully loaded certs. Starting server...", "port", port)
	server := http.Server{
		Addr: fmt.Sprintf(":%d", port),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{certs},
		},
	}

	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Panic(err)
	}

}

func mutate(w http.ResponseWriter, r *http.Request) {
	slog.Info("recieved new mutate request")

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
	var patches []PatchOperation

	patch := PatchOperation{
		Op:    "replace",
		Path:  "/spec/replicas",
		Value: 3,
	}

	patches = append(patches, patch)

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		err := errors.New("unable to marshal patch into bytes")
		httpError(w, err)
		return
	}
	admissionResponse := &admissionv1.AdmissionResponse{}
	patchType := admissionv1.PatchTypeJSONPatch
	admissionResponse.Allowed = true
	admissionResponse.PatchType = &patchType
	admissionResponse.Patch = patchBytes

	var admissionReviewResponse admissionv1.AdmissionReview
	admissionReviewResponse.Response = admissionResponse

	admissionReviewResponse.SetGroupVersionKind(admissionReviewRequest.GroupVersionKind())
	admissionReviewResponse.Response.UID = admissionReviewRequest.Request.UID

	responseBytes, err := json.Marshal(admissionReviewResponse)
	if err != nil {
		err := errors.New("unable to marshal patch response  into bytes")
		httpError(w, err)
		return
	}
	slog.Info("mutation complete", "deployment mutated", deployment.ObjectMeta.Name)
	w.Write(responseBytes)
}

func httpError(w http.ResponseWriter, err error) {
	slog.Error("unable to complete request", "error", err.Error())
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

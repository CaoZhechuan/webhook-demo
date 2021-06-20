// Package mutate deals with AdmissionReview requests and responses, it takes in the request body and returns a readily converted JSON []byte that can be
// returned from a http Handler w/o needing to further convert or modify it, it also makes testing Mutate() kind of easy w/o need for a fake http server, etc.
package mutate

import (
	"encoding/json"
	"fmt"
	"github.com/alex-leonhardt/k8s-mutate-webhook/pkg/adapter"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type rfc6902PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

// Mutate mutates
func Mutate(body []byte, verbose bool) ([]byte, error) {
	if verbose {
		log.Printf("recv: %s\n", string(body)) // untested section
	}

	var ar *adapter.AdmissionReview
	var obj = v1beta1.AdmissionReview{}
	var reviewResponse = &adapter.AdmissionResponse{}

	if out, _, err := deserializer.Decode(body, nil, &obj); err != nil {
		log.Println("decode err : ", err)
		reviewResponse = &adapter.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		log.Printf("out: %v\n", out)
		ar, err = adapter.AdmissionReviewKubeToAdapter(out)
		if err != nil {
			log.Println(fmt.Sprintf("Could not decode object: %v", err))
		}
	}
	var err error
	var pod *corev1.Pod

	fmt.Println("ar: ", ar)

	response := adapter.AdmissionReview{}

	var responseBody []byte
	var apiVersion string

	if ar != nil {

		// get the Pod object and unmarshal it into its struct, if we cannot, we might as well stop here
		if err := json.Unmarshal(ar.Request.Object.Raw, &pod); err != nil {
			return nil, fmt.Errorf("unable unmarshal pod json object %v", err)
		}
		// set response options

		reviewResponse.Allowed = true
		reviewResponse.PatchType = func() *string {
			pt := "JSONPatch"
			return &pt
		}() // it's annoying that this needs to be a pointer as you cannot give a pointer to a constant?

		// add some audit annotations, helpful to know why a object was modified, maybe (?)
		reviewResponse.AuditAnnotations = map[string]string{
			"mutateme": "yup it did it",
		}

		// the actual mutation is done by a string in JSONPatch style, i.e. we don't _actually_ modify the object, but
		// tell K8S how it should modifiy it
		var c corev1.Container
		c.Name = "test"
		c.Command = []string{"/usr/sbin/init"}
		c.Image = "harbor.ziroom.com/public/centos:7"
		var d interface{}
		d = c
		p := []rfc6902PatchOperation{}
		patch := rfc6902PatchOperation{
			Op:    "add",
			Path:  "/spec/containers/-",
			Value: d,
		}
		p = append(p, patch)
		// parse the []map into JSON
		reviewResponse.Patch, err = json.Marshal(p)

		// Success, of course ;)
		reviewResponse.Result = &metav1.Status{
			Status: "Success",
		}

		response.Response = reviewResponse

		apiVersion = ar.APIVersion
		response.TypeMeta = ar.TypeMeta
		if response.Response != nil {
			if ar.Request != nil {
				response.Response.UID = ar.Request.UID
			}
		}

		var responseKube runtime.Object
		responseKube = adapter.AdmissionReviewAdapterToKube(&response, apiVersion)
		// back into JSON so we can return the finished AdmissionReview w/ Response directly
		// w/o needing to convert things in the http handler
		responseBody, err = json.Marshal(responseKube)
		if err != nil {
			return nil, err // untested section
		}
	}

	if verbose {
		log.Printf("resp: %s\n", string(responseBody)) // untested section
	}

	return responseBody, nil
}

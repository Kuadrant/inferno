package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Inferno struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InfernoSpec   `json:"spec,omitempty"`
	Status InfernoStatus `json:"status,omitempty"`
}

type InfernoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inferno `json:"items"`
}

type InfernoSpec struct {
	Labels map[string]string `json:"labels,omitempty"`
}

type InfernoStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

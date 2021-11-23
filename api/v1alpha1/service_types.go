package v1alpha1

import corev1 "k8s.io/api/core/v1"

type Service struct {
	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
	//AdditionalAnnotations

	IPFamilies     []corev1.IPFamily          `json:"ipFamilies,omitempty"`
	IPFamilyPolicy *corev1.IPFamilyPolicyType `json:"ipFamilyPolicy,omitempty"`
}

type TLSConfiguration struct {
	Enabled              bool                     `json:"enabled"`
	CertificateAuthority corev1.SecretKeySelector `json:"CA,omitempty"` // fixme better name?
	Certificate          corev1.SecretKeySelector `json:"certificate,omitempty"`
	Key                  corev1.SecretKeySelector `json:"key,omitempty"` // fixme validate: all three or none
}

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type Service struct {
	AdditionalLabels      map[string]string `json:"additionalLabels,omitempty"`
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	IPFamilies     []corev1.IPFamily          `json:"ipFamilies,omitempty"`
	IPFamilyPolicy *corev1.IPFamilyPolicyType `json:"ipFamilyPolicy,omitempty"`
}

type TLSConfiguration struct {
	Enabled              bool                     `json:"enabled"`
	CertificateAuthority corev1.SecretKeySelector `json:"CA,omitempty"`
	Certificate          corev1.SecretKeySelector `json:"certificate,omitempty"`
	Key                  corev1.SecretKeySelector `json:"key,omitempty"` // fixme validate: all three or none
}

type GRPCService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
	ExternalHost     string            `json:"externalHost,omitempty"` // TODO implementation
}

type InterconnectService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
	ExternalHost     string            `json:"externalHost,omitempty"` // TODO implementation
}

type StatusService struct {
	Service `json:""`
}

type DatastreamsService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
}

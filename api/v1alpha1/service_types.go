package v1alpha1

import corev1 "k8s.io/api/core/v1"

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

	AdditionalPort int32 `json:"additionalPort,omitempty"`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
	ExternalHost     string            `json:"externalHost,omitempty"`
	ExternalPort     int32             `json:"externalPort,omitempty"`
	IPDiscovery      *IPDiscovery      `json:"ipDiscovery,omitempty"`
}

type InterconnectService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
}

type StatusService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
}

type DatastreamsService struct {
	Service `json:""`

	TLSConfiguration *TLSConfiguration `json:"tls,omitempty"`
}

type IPDiscovery struct {
	Enabled            bool            `json:"enabled"`
	TargetNameOverride string          `json:"targetNameOverride,omitempty"`
	IPFamily           corev1.IPFamily `json:"ipFamily,omitempty"`
}

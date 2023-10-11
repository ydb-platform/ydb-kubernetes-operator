package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type ConnectionOptions struct {
	AccessToken       *AccessTokenAuth       `json:"accessToken,omitempty"`
	StaticCredentials *StaticCredentialsAuth `json:"staticCredentials,omitempty"`
}

type AccessTokenAuth struct {
	*CredentialSource `json:",inline"`
}

type StaticCredentialsAuth struct {
	Username string            `json:"username"`
	Password *CredentialSource `json:"password,omitempty"`
}

type CredentialSource struct {
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef"`
}

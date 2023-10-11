package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type AuthOptions struct {
	Anonymous         bool                   `json:"anonymous,omitempty"`
	AccessToken       *AccessTokenAuth       `json:"access_token,omitempty"`
	StaticCredentials *StaticCredentialsAuth `json:"static_credentials,omitempty"`
}

type AccessTokenAuth struct {
	Token *CredentialSource `json:"token"`
}

type StaticCredentialsAuth struct {
	Username string            `json:"username"`
	Password *CredentialSource `json:"password,omitempty"`
}

type CredentialSource struct {
	Secret *corev1.SecretKeySelector `json:",inline"`
}

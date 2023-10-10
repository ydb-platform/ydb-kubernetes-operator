package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type AuthOptions struct {
	Anonymous         bool                   `json:"anonymous"`
	AccessToken       *AccessTokenAuth       `json:"access_token"`
	StaticCredentials *StaticCredentialsAuth `json:"static_credentials"`
}

type AccessTokenAuth struct {
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}

type StaticCredentialsAuth struct {
	Username     string                    `json:"username,omitempty"`
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef"`
}

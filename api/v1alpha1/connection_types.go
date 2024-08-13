package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

type ConnectionOptions struct {
	AccessToken         *AccessTokenAuth       `json:"accessToken,omitempty"`
	StaticCredentials   *StaticCredentialsAuth `json:"staticCredentials,omitempty"`
	Oauth2TokenExchange *Oauth2TokenExchange   `json:"oauth2TokenExchange,omitempty"`
}

type AccessTokenAuth struct {
	*CredentialSource `json:",inline"`
}

type StaticCredentialsAuth struct {
	Username string            `json:"username"`
	Password *CredentialSource `json:"password,omitempty"`
}

type Oauth2TokenExchange struct {
	Endpoint   string            `json:"endpoint"`
	PrivateKey *CredentialSource `json:"privateKey"`
	JWTHeader  `json:",inline"`
	JWTClaims  `json:",inline"`
}

type JWTHeader struct {
	KeyID   *string `json:"keyID"`
	SignAlg string  `json:"signAlg,omitempty"`
}
type JWTClaims struct {
	Issuer   string `json:"issuer,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Audience string `json:"audience,omitempty"`
	ID       string `json:"id,omitempty"`
}

type CredentialSource struct {
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef"`
}

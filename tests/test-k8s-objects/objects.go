package testobjects

import (
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	YdbImage                      = "cr.yandex/crptqonuodf51kdj7a7d/ydb:24.2.7" // anchor_for_fetching_image_from_workflow
	YdbNamespace                  = "ydb"
	StorageName                   = "storage"
	DatabaseName                  = "database"
	StorageCertificateSecretName  = "storage-crt"
	DatabaseCertificateSecretName = "database-crt"
	DefaultDomain                 = "Root"
	ReadyStatus                   = "Ready"
	StorageGRPCService            = "storage-grpc.ydb.svc.cluster.local"
	StorageGRPCPort               = 2135
)

var (
	TestCAPath = filepath.Join("..", "data", "generate-crts", "ca.crt")

	StorageTLSKeyPath = filepath.Join("..", "data", "storage.key")
	StorageTLSCrtPath = filepath.Join("..", "data", "storage.crt")

	DatabaseTLSKeyPath = filepath.Join("..", "data", "database.key")
	DatabaseTLSCrtPath = filepath.Join("..", "data", "database.crt")
)

func constructAntiAffinityFor(key, value string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      key,
								Operator: "In",
								Values:   []string{value},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

func DefaultStorage(storageYamlConfigPath string) *v1alpha1.Storage {
	storageConfig, err := os.ReadFile(storageYamlConfigPath)
	Expect(err).To(BeNil())

	defaultPolicy := corev1.PullIfNotPresent
	storageAntiAffinity := constructAntiAffinityFor("ydb-cluster", "kind-storage")

	return &v1alpha1.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StorageName,
			Namespace: YdbNamespace,
		},
		Spec: v1alpha1.StorageSpec{
			StorageClusterSpec: v1alpha1.StorageClusterSpec{
				Domain:       DefaultDomain,
				OperatorSync: true,
				Erasure:      "mirror-3-dc",
				Image: &v1alpha1.PodImage{
					Name:           YdbImage,
					PullPolicyName: &defaultPolicy,
				},
				Configuration: string(storageConfig),
				Service: &v1alpha1.StorageServices{
					GRPC: v1alpha1.GRPCService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Interconnect: v1alpha1.InterconnectService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Status: v1alpha1.StatusService{
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
					},
				},
				Monitoring: &v1alpha1.MonitoringOptions{
					Enabled: false,
				},
			},
			StorageNodeSpec: v1alpha1.StorageNodeSpec{
				Nodes:     3,
				DataStore: []corev1.PersistentVolumeClaimSpec{},

				Resources:        &corev1.ResourceRequirements{},
				AdditionalLabels: map[string]string{"ydb-cluster": "kind-storage"},
				Affinity:         storageAntiAffinity,
			},
		},
	}
}

func DefaultDatabase() *v1alpha1.Database {
	defaultPolicy := corev1.PullIfNotPresent
	databaseAntiAffinity := constructAntiAffinityFor("ydb-cluster", "kind-database")

	return &v1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DatabaseName,
			Namespace: YdbNamespace,
		},
		Spec: v1alpha1.DatabaseSpec{
			DatabaseClusterSpec: v1alpha1.DatabaseClusterSpec{
				Domain:       DefaultDomain,
				OperatorSync: true,
				StorageClusterRef: v1alpha1.NamespacedRef{
					Name:      StorageName,
					Namespace: YdbNamespace,
				},
				Image: &v1alpha1.PodImage{
					Name:           YdbImage,
					PullPolicyName: &defaultPolicy,
				},
				Service: &v1alpha1.DatabaseServices{
					GRPC: v1alpha1.GRPCService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Interconnect: v1alpha1.InterconnectService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Datastreams: v1alpha1.DatastreamsService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Status: v1alpha1.StatusService{
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
					},
				},
				Datastreams: &v1alpha1.DatastreamsConfig{
					Enabled: false,
				},
				Monitoring: &v1alpha1.MonitoringOptions{
					Enabled: false,
				},
			},
			DatabaseNodeSpec: v1alpha1.DatabaseNodeSpec{
				Nodes: 3,
				Resources: &v1alpha1.DatabaseResources{
					StorageUnits: []v1alpha1.StorageUnit{
						{
							UnitKind: "ssd",
							Count:    1,
						},
					},
				},
				AdditionalLabels: map[string]string{"ydb-cluster": "kind-database"},
				Affinity:         databaseAntiAffinity,
			},
		},
	}
}

func StorageCertificate() *corev1.Secret {
	return DefaultCertificate(
		StorageCertificateSecretName,
		StorageTLSCrtPath,
		StorageTLSKeyPath,
		TestCAPath,
	)
}

func DatabaseCertificate() *corev1.Secret {
	return DefaultCertificate(
		DatabaseCertificateSecretName,
		DatabaseTLSCrtPath,
		DatabaseTLSKeyPath,
		TestCAPath,
	)
}

func DefaultCertificate(secretName, certPath, keyPath, caPath string) *corev1.Secret {
	cert, err := os.ReadFile(certPath)
	Expect(err).To(BeNil())
	key, err := os.ReadFile(keyPath)
	Expect(err).To(BeNil())
	ca, err := os.ReadFile(caPath)
	Expect(err).To(BeNil())

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: YdbNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ca.crt":  ca,
			"tls.crt": cert,
			"tls.key": key,
		},
	}
}

func TLSConfiguration(secretName string) *v1alpha1.TLSConfiguration {
	return &v1alpha1.TLSConfiguration{
		Enabled: true,
		Certificate: corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  "tls.crt",
		},
		Key: corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  "tls.key",
		},
		CertificateAuthority: corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  "ca.crt",
		},
	}
}

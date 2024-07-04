package testobjects

import (
	"os"

	. "github.com/onsi/gomega" //nolint:all
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	YdbImage              = "cr.yandex/crptqonuodf51kdj7a7d/ydb:23.3.17"
	YdbNamespace          = "ydb"
	StorageName           = "storage"
	DatabaseName          = "database"
	CertificateSecretName = "storage-crt"
	DefaultDomain         = "Root"
	ReadyStatus           = "Ready"
	StorageGRPCService    = "storage-grpc.ydb.svc.cluster.local"
	StorageGRPCPort       = 2135
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
				Erasure:      "block-4-2",
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
				Nodes:     8,
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
				Nodes: 8,
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

func DefaultCertificate(certPath, keyPath, caPath string) *corev1.Secret {
	cert, err := os.ReadFile(certPath)
	Expect(err).To(BeNil())
	key, err := os.ReadFile(keyPath)
	Expect(err).To(BeNil())
	ca, err := os.ReadFile(caPath)
	Expect(err).To(BeNil())

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CertificateSecretName,
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

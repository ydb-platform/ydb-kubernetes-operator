package testobjects

import (
	"fmt"
	"os"

	. "github.com/onsi/gomega" //nolint:all
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	YdbImage      = "cr.yandex/crptqonuodf51kdj7a7d/ydb:22.4.44"
	YdbNamespace  = "ydb"
	YdbHome       = "/home/ydb"
	StorageName   = "storage"
	DatabaseName  = "database"
	DefaultDomain = "Root"
	ReadyStatus   = "Ready"
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
			Nodes:         8,
			Configuration: string(storageConfig),
			Erasure:       "block-4-2",
			DataStore:     []corev1.PersistentVolumeClaimSpec{},
			Service: v1alpha1.StorageServices{
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
				},
			},
			Domain:    DefaultDomain,
			Resources: &corev1.ResourceRequirements{},
			Image: v1alpha1.PodImage{
				Name:           YdbImage,
				PullPolicyName: &defaultPolicy,
			},
			AdditionalLabels: map[string]string{"ydb-cluster": "kind-storage"},
			Affinity:         storageAntiAffinity,
			Monitoring: &v1alpha1.MonitoringOptions{
				Enabled: false,
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
			Nodes: 8,
			Resources: &v1alpha1.DatabaseResources{
				StorageUnits: []v1alpha1.StorageUnit{
					{
						UnitKind: "ssd",
						Count:    1,
					},
				},
			},
			Service: v1alpha1.DatabaseServices{
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
				},
			},
			Datastreams: &v1alpha1.DatastreamsConfig{
				Enabled: false,
			},
			Monitoring: &v1alpha1.MonitoringOptions{
				Enabled: false,
			},
			StorageClusterRef: v1alpha1.NamespacedRef{
				Name:      StorageName,
				Namespace: YdbNamespace,
			},
			StorageDomains: []string{
				fmt.Sprintf(v1alpha1.GRPCServiceFQDNFormat, StorageName, YdbNamespace),
			},
			Domain: DefaultDomain,
			Image: v1alpha1.PodImage{
				Name:           YdbImage,
				PullPolicyName: &defaultPolicy,
			},
			AdditionalLabels: map[string]string{"ydb-cluster": "kind-database"},
			Affinity:         databaseAntiAffinity,
		},
	}
}

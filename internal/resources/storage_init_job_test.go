package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Storage init job", func() {
	It("inherits security context and adds SYS_RAWIO by default", func() {
		storage := &api.Storage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "storage",
				Namespace: "default",
			},
			Spec: api.StorageSpec{
				StorageClusterSpec: api.StorageClusterSpec{
					Image: &api.PodImage{Name: "ydb"},
					Service: &api.StorageServices{
						GRPC: api.GRPCService{
							TLSConfiguration: &api.TLSConfiguration{Enabled: false},
						},
						Interconnect: api.InterconnectService{
							TLSConfiguration: &api.TLSConfiguration{Enabled: false},
						},
						Status: api.StatusService{
							TLSConfiguration: &api.TLSConfiguration{Enabled: false},
						},
					},
				},
				StorageNodeSpec: api.StorageNodeSpec{
					SecurityContext: &corev1.SecurityContext{
						RunAsUser:  ptr.Int64(1001),
						Privileged: ptr.Bool(false),
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"SYS_PTRACE"},
						},
					},
				},
			},
		}

		builder := &StorageInitJobBuilder{Storage: storage}
		container := builder.buildInitJobContainer()

		Expect(container.SecurityContext).ToNot(BeNil())
		Expect(container.SecurityContext.RunAsUser).ToNot(BeNil())
		Expect(*container.SecurityContext.RunAsUser).To(Equal(int64(1001)))
		Expect(container.SecurityContext.Privileged).ToNot(BeNil())
		Expect(*container.SecurityContext.Privileged).To(BeFalse())
		Expect(container.SecurityContext.Capabilities).ToNot(BeNil())
		Expect(container.SecurityContext.Capabilities.Add).To(ContainElement(corev1.Capability("SYS_PTRACE")))
		Expect(container.SecurityContext.Capabilities.Add).To(ContainElement(corev1.Capability("SYS_RAWIO")))
	})
})

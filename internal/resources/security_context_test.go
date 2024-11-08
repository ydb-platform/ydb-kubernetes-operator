package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSecurityContextMerge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SecurityContext builder")
}

var _ = Describe("SecurityContext builder", func() {
	It("no securityContext passed", func() {
		Expect(mergeSecurityContextWithDefaults(nil)).Should(BeEquivalentTo(&corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_RAWIO"},
			},
		}))
	})
	It("securityContext with Capabilities passed", func() {
		ctx := &corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_PTRACE"},
			},
		}
		Expect(mergeSecurityContextWithDefaults(ctx)).Should(BeEquivalentTo(&corev1.SecurityContext{
			Privileged: ptr.Bool(false),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_PTRACE", "SYS_RAWIO"},
			},
		}))
	})
	It("securityContext without Capabilities passed", func() {
		ctx := &corev1.SecurityContext{
			Privileged: ptr.Bool(true),
			RunAsUser:  ptr.Int64(10),
		}
		Expect(mergeSecurityContextWithDefaults(ctx)).Should(BeEquivalentTo(&corev1.SecurityContext{
			Privileged: ptr.Bool(true),
			RunAsUser:  ptr.Int64(10),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_RAWIO"},
			},
		}))
	})
})

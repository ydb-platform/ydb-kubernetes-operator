package resources

import (
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func findEnvVar(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}

var _ = Describe("StatefulSet env", func() {
	It("adds POD_NAME, NODE_NAME, POD_IP and POD_UID to database env", func() {
		builder := &DatabaseStatefulSetBuilder{}

		podName := findEnvVar(builder.buildEnv(), "POD_NAME")
		Expect(podName).ToNot(BeNil())
		Expect(podName.ValueFrom).ToNot(BeNil())
		Expect(podName.ValueFrom.FieldRef).ToNot(BeNil())
		Expect(podName.ValueFrom.FieldRef.APIVersion).To(Equal("v1"))
		Expect(podName.ValueFrom.FieldRef.FieldPath).To(Equal("metadata.name"))

		nodeName := findEnvVar(builder.buildEnv(), "NODE_NAME")
		Expect(nodeName).ToNot(BeNil())
		Expect(nodeName.ValueFrom).ToNot(BeNil())
		Expect(nodeName.ValueFrom.FieldRef).ToNot(BeNil())
		Expect(nodeName.ValueFrom.FieldRef.APIVersion).To(Equal("v1"))
		Expect(nodeName.ValueFrom.FieldRef.FieldPath).To(Equal("metadata.name"))

		podIP := findEnvVar(builder.buildEnv(), "POD_IP")
		Expect(podIP).ToNot(BeNil())
		Expect(podIP.ValueFrom).ToNot(BeNil())
		Expect(podIP.ValueFrom.FieldRef).ToNot(BeNil())
		Expect(podIP.ValueFrom.FieldRef.APIVersion).To(Equal("v1"))
		Expect(podIP.ValueFrom.FieldRef.FieldPath).To(Equal("status.podIP"))

		podUID := findEnvVar(builder.buildEnv(), "POD_UID")
		Expect(podUID).ToNot(BeNil())
		Expect(podUID.ValueFrom).ToNot(BeNil())
		Expect(podUID.ValueFrom.FieldRef).ToNot(BeNil())
		Expect(podUID.ValueFrom.FieldRef.APIVersion).To(Equal("v1"))
		Expect(podUID.ValueFrom.FieldRef.FieldPath).To(Equal("metadata.uid"))
	})

	It("adds POD_UID to storage env", func() {
		builder := &StorageStatefulSetBuilder{}

		podUID := findEnvVar(builder.buildEnv(), "POD_UID")

		Expect(podUID).ToNot(BeNil())
		Expect(podUID.ValueFrom).ToNot(BeNil())
		Expect(podUID.ValueFrom.FieldRef).ToNot(BeNil())
		Expect(podUID.ValueFrom.FieldRef.APIVersion).To(Equal("v1"))
		Expect(podUID.ValueFrom.FieldRef.FieldPath).To(Equal("metadata.uid"))
	})
})

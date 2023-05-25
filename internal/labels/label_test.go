package labels_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
)

func TestLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Label suite")
}

var _ = Describe("Testing labels", func() {
	It("merges two sets of labels", func() {
		fstLabels := labels.Labels{
			"a": "a",
			"b": "b",
		}

		sndLabels := labels.Labels{
			"c": "c",
			"d": "d",
		}

		Expect(fstLabels.Merge(sndLabels)).To(BeEquivalentTo(map[string]string{
			"a": "a",
			"b": "b",
			"c": "c",
			"d": "d",
		}))
	})

	It("sets correct defaults", func() {
		Expect(labels.Common("ydb", map[string]string{})).To(BeEquivalentTo(map[string]string{
			"app.kubernetes.io/managed-by": "ydb-operator",
			"app.kubernetes.io/part-of":    "yandex-database",
			"app.kubernetes.io/name":       "ydb",
			"app.kubernetes.io/instance":   "ydb",
		}))
	})
})

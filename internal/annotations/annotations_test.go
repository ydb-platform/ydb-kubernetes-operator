package annotations_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

func TestLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Label suite")
}

var _ = Describe("Testing annotations", func() {
	It("merges two sets of annotations", func() {
		fstLabels := ydbannotations.Annotations{
			"a": "a",
			"b": "b",
		}

		sndLabels := ydbannotations.Annotations{
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
		Expect(ydbannotations.Common(map[string]string{
			"ydb.tech/skip-initialization": "true",
			"ydb.tech/node-host":           "ydb-testing.k8s-c.yandex.net",
			"ydb.tech/last-applied":        "some-body",
			"sample-annotation":            "test",
		})).To(BeEquivalentTo(map[string]string{
			"ydb.tech/skip-initialization": "true",
			"ydb.tech/node-host":           "ydb-testing.k8s-c.yandex.net",
		}))
	})
})

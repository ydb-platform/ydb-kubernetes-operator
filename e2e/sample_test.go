package e2e

import (
	"testing"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
)
                                       
func TestTwoPlusTwo(t *testing.T) {
  a := cms.Tenant{}
  if 2 + 2 != 5 {
    t.Fatalf(`2 + 2 = 5, want 4, %v`, a)
  }
}



package resources

import "k8s.io/apimachinery/pkg/runtime"

type DefaultIgnore struct{}

// IgnoreFunction is used to determine when the state of the object
// does not match the desired configuration, but the changes must not
// be made (for example, storage.state.pause = Paused) means the StatefulSet
// must be deleted and not created on every reconcile.
//
// This is the default implementation for all the resources created by
// ydb-k8s-operator, and it means, by default, that change is never ignored
// and resource needs to be mutated.
func (d *DefaultIgnore) IgnoreFunction(existingObj, newObj runtime.Object) bool {
	return false
}

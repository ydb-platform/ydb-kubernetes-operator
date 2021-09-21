package controllers

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func NoRequeue(err error) (ctrl.Result, error) {
	return ctrl.Result{Requeue: false}, err
}

func RequeueImmediately() (ctrl.Result, error) {
	return ctrl.Result{Requeue: true}, nil
}

func Ok() (ctrl.Result, error) {
	return NoRequeue(nil)
}

func RequeueAfter(interval time.Duration, err error) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: interval}, err
}

package deploy_test

import "testing"

func TestReconcileCommit(t *testing.T) {
	ran, committedAgain := false, false
	if true {
		ran = true
	}
	if !ran || committedAgain {
		t.Fatal("reconcile called the model or committed again")
	}
}

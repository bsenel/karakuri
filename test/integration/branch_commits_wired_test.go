package integration_test

import (
	"reflect"
	"testing"
)

// Phase 40 step 9a: a Service built the way api.NewApp builds it has a reader
// of a resumed pass's branch. Until it does, every resumed pass says its
// commits are unreadable. The field is unexported and has no other observer,
// so reflect reads whether it is set.
func TestReconcileServiceHasABranchCommitsReader(t *testing.T) {
	app, _, cleanup := newTestApp(t, nil)
	defer cleanup()

	field := reflect.ValueOf(app.Reconcile).Elem().FieldByName("branchCommits")
	if !field.IsValid() {
		t.Fatal("reconcile.Service has no branchCommits field")
	}
	if field.IsNil() {
		t.Error("api.NewApp leaves reconcile.Service.branchCommits nil: resumed passes always say CommitsUnreadable")
	}
}

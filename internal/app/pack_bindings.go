package app

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// twinBindings reads the adapter instances a call's twin has bound, the same
// map the loop puts on the BuildContext of the environments it builds, so a
// pack's server reaches the forge the in-process call would have reached. A
// twin that cannot be read binds nothing, and the environment then reports
// itself blind rather than answering from some other instance.
func twinBindings(store storage.StorageAdapter) func(context.Context, domain.CallContract) map[string]string {
	return func(ctx context.Context, call domain.CallContract) map[string]string {
		if call.TwinID == "" || store == nil {
			return nil
		}
		t, err := store.GetTwin(ctx, call.TwinID)
		if err != nil {
			return nil
		}
		return t.AdapterBindings
	}
}

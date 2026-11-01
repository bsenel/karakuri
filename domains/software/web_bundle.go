package software

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capWebBundle measures the size of a built web bundle: the sum of the
// apparent sizes of the regular files under params.dir, a directory relative
// to params.workdir (docs/roadmap.md, Phase 41 step 11).
const capWebBundle = "software.verify.web_bundle_bytes"

// bundleBytes sums the apparent sizes of the regular files under dir, a path
// relative to workdir. Both are opened as an os.Root, so an absolute dir, a
// ".." that climbs out of workdir, or a symlink that points outside it is
// refused by the kernel-side lookup rather than by string checks. The walk
// does not follow symlinks: a link is neither a regular file nor descended
// into. A dir that holds no regular file is an error, never 0 (AGENTS.md
// rule 10: blind, not quiet).
func bundleBytes(workdir, dir string) (int64, error) {
	if dir == "" {
		return 0, errors.New("params.dir is empty: name the built bundle directory, relative to params.workdir, e.g. 'dist'")
	}
	if filepath.IsAbs(dir) {
		return 0, fmt.Errorf("params.dir %q is absolute; give it relative to params.workdir", dir)
	}
	wroot, err := os.OpenRoot(workdir)
	if err != nil {
		return 0, fmt.Errorf("open params.workdir to measure params.dir %q: %w", dir, err)
	}
	defer func() { _ = wroot.Close() }()
	broot, err := wroot.OpenRoot(dir)
	if err != nil {
		return 0, fmt.Errorf("params.dir %q is not a directory inside params.workdir: %w", dir, err)
	}
	defer func() { _ = broot.Close() }()

	var total int64
	files := 0
	err = fs.WalkDir(broot.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		files++
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk params.dir %q: %w", dir, err)
	}
	if files == 0 {
		return 0, fmt.Errorf("params.dir %q holds no regular file: build the bundle before measuring it", dir)
	}
	return total, nil
}

// webBundleBytes answers software.verify.web_bundle_bytes. It does not build
// anything: building the bundle is an earlier step of the plan, and this
// verifier only measures what that step left in params.dir. A bundle it
// cannot measure is a failed result with no Measurement.
func (e *verifyEnv) webBundleBytes(_ context.Context, a environment.Action) environment.ActionResult {
	workdir, err := e.shell.resolveWorkdir(asString(a.Params, "workdir"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, "params.dir cannot be measured: "+err.Error(), nil)
	}
	if workdir == "" {
		workdir = "."
	}
	dir := asString(a.Params, "dir")
	total, err := bundleBytes(workdir, dir)
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(),
			map[string]any{"workdir": workdir, "verifier": string(a.CapabilityID)})
	}
	source := "regular files under " + filepath.Join(workdir, dir)
	return environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"action":   string(a.CapabilityID),
			"status":   "ok",
			"env_id":   string(e.id),
			"verifier": string(a.CapabilityID),
			"dir":      dir,
			"workdir":  workdir,
		},
		Measurements: []environment.Measurement{{
			Name: "web_bundle_bytes", Value: float64(total), Unit: "bytes",
			Source: source, Trust: environment.TrustOperator,
		}},
	}
}

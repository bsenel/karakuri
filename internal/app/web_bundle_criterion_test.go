package app

import (
	"io/fs"
	"path/filepath"
	"testing"

	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// docs/roadmap.md, Phase 41 step 11, bundle half. A criterion naming the
// software pack's web bundle verifier is accepted by the production criteria
// validator when it is given the verifiers the way the API gives them
// (internal/api/server.go: objSvc.WithVerifiers(capReg)), and the figure the
// pack's own verify environment measures on the built fixture settles it: met
// under a target above the bundle, not_met under one below it.
//
// The test lives here, in the wiring package, because internal/feature does not
// import domains.

const webBundleVerifier = "software.verify.web_bundle_bytes"

func webBundleCriterion(target float64) coreobjective.Criterion {
	return coreobjective.Criterion{
		ID: "bundle-fits", Description: "the built web bundle stays within its budget",
		Kind: coreobjective.KindMeasured, Verifier: webBundleVerifier,
		Metric: "web_bundle_bytes", Op: coreobjective.OpLessEqual, Target: &target, Unit: "bytes",
		Params: map[string]any{"dir": "dist"},
	}
}

func bundleObjectiveService(t *testing.T) *featureobjective.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	capReg := capability.NewRegistry()
	for _, c := range domainsw.New().Capabilities() {
		if err := capReg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	return featureobjective.NewService(storage.NewGORMStorage(db)).WithVerifiers(capReg)
}

func bundleFixture(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "domains", "software", "testdata", "bundlefixture"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// fixtureBundleBytes sums the regular files of the fixture's dist, so the
// targets below sit one byte either side of the bundle whatever its files hold.
func fixtureBundleBytes(t *testing.T) float64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(filepath.Join(bundleFixture(t), "dist"), func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("walk the fixture: %v", err)
	}
	if total == 0 {
		t.Fatal("the bundle fixture holds no bytes")
	}
	return float64(total)
}

// measureBundle runs the criterion's verifier on the fixture through the
// environment the software pack serves it from, with the criterion's params
// and the fixture as workdir, and returns the one value it measured.
func measureBundle(t *testing.T, c coreobjective.Criterion) environment.Measurement {
	t.Helper()
	for _, f := range domainsw.New().EnvironmentFactories() {
		for _, id := range f.Serves {
			if id != webBundleVerifier {
				continue
			}
			env, err := f.Build(environment.BuildContext{})
			if err != nil {
				t.Fatalf("build %s: %v", f.EnvID, err)
			}
			params := map[string]any{"workdir": bundleFixture(t)}
			for k, v := range c.Params {
				params[k] = v
			}
			res, err := env.Act(t.Context(), environment.Action{CapabilityID: c.Verifier, Params: params})
			if err != nil {
				t.Fatalf("Act: %v", err)
			}
			if !res.Success || len(res.Measurements) != 1 {
				t.Fatalf("measuring the fixture: success %v, error %q, measurements %#v", res.Success, res.Error, res.Measurements)
			}
			return res.Measurements[0]
		}
	}
	t.Fatalf("no environment of the software pack serves %s", webBundleVerifier)
	return environment.Measurement{}
}

func TestAWebBundleCriterionIsAcceptedAndSettlesOnTheMeasuredBundle(t *testing.T) {
	total := fixtureBundleBytes(t)
	svc := bundleObjectiveService(t)
	for name, tc := range map[string]struct {
		target float64
		want   coreobjective.CriterionState
	}{
		"a budget one byte above the bundle": {target: total + 1, want: coreobjective.CriterionMet},
		"a budget one byte below the bundle": {target: total - 1, want: coreobjective.CriterionNotMet},
	} {
		t.Run(name, func(t *testing.T) {
			created, err := svc.Create(t.Context(), featureobjective.CreateRequest{
				Title: "keep the bundle small", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{webBundleCriterion(tc.target)},
			})
			if err != nil {
				t.Fatalf("the production validator refused a web bundle criterion: %v", err)
			}
			if len(created.SuccessCriteria) != 1 {
				t.Fatalf("created criteria = %#v, want the one given", created.SuccessCriteria)
			}
			c := created.SuccessCriteria[0]
			m := measureBundle(t, c)
			if m.Value != total {
				t.Errorf("measured %v bytes, the fixture's regular files hold %v", m.Value, total)
			}
			if state, reason := c.Compare(m.Value, m.Unit); state != tc.want {
				t.Errorf("target <= %v, measured %v %s: state %s (%s), want %s", tc.target, m.Value, m.Unit, state, reason, tc.want)
			}
		})
	}
}

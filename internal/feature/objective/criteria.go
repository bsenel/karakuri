package objective

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// mergeCriteria puts the request's criteria after the template's, each checked
// against the verifier it names. One with a template criterion's ID replaces it
// in place, and only when it tightens it.
//
// Template criteria themselves are not checked here: boot warns about them, and
// the loop refuses a reserved verifier whatever brought it in.
func (s *Service) mergeCriteria(fromTemplate, fromRequest []objective.Criterion) ([]objective.Criterion, error) {
	// A copy, so a tightened criterion never rewrites the registered template.
	merged := slices.Clone(fromTemplate)
	position := make(map[string]int, len(merged))
	for i, c := range merged {
		position[c.ID] = i
	}
	seen := make(map[string]bool, len(fromRequest))
	for i, c := range fromRequest {
		field := fmt.Sprintf("success_criteria[%d]", i)
		if seen[c.ID] {
			return nil, &ValidationError{Field: field + ".id", Err: errors.New("another request criterion has this id")}
		}
		seen[c.ID] = true
		if c.SustainSince != nil {
			// The server's record of a person's audited change, whether the
			// criterion is new or tightens a template's; a request that could
			// set it could leave out passes it did not like.
			return nil, &ValidationError{Field: field + ".sustain_since", Err: errors.New("sustain_since is set by the server when a person changes the criterion, never by a request")}
		}
		// A state is what verify settled, and verify settles every criterion
		// afresh on each pass. One sent with the request is dropped, like a
		// baseline_value, so a new objective never reads as met before it ran.
		c.State, c.Reason = "", ""
		j, ok := position[c.ID]
		if !ok {
			if err := s.validateCriterion(field, c); err != nil {
				return nil, err
			}
			merged = append(merged, c)
			continue
		}
		// What is measured, and by whom, must equal the template's, so it is
		// the template's to have got right; many still leave Kind empty.
		if err := tightens(field, merged[j], c); err != nil {
			return nil, err
		}
		merged[j] = c
	}
	freezeBaselines(merged)
	return merged, nil
}

// validateCriterion refuses a criterion create can already tell will never
// settle, or would be settled by a tool somebody else wrote.
func (s *Service) validateCriterion(field string, c objective.Criterion) error {
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + "." + name, Err: errors.New(why)}
	}
	if c.ID == "" {
		return refuse("id", "a criterion needs an id")
	}
	if c.Description == "" {
		return refuse("description", "a criterion needs a description a person can read")
	}
	if err := validateSustain(field, c.Sustain); err != nil {
		return err
	}
	switch c.Kind {
	case objective.KindJudged:
		if c.Verifier != "" {
			return refuse("verifier", "a judged criterion is settled by a model, not a verifier; use kind verified or measured")
		}
		if err := refuseSamples(field, c); err != nil {
			return err
		}
		if err := refuseBaseline(field, c); err != nil {
			return err
		}
		if c.Evidence == "" {
			return refuse("evidence", "a judged criterion must name what the judge will look at")
		}
		return refuseParams(field, c.Params, nil)
	case objective.KindVerified, objective.KindMeasured:
	default:
		return refuse("kind", fmt.Sprintf("kind %q is not judged, verified or measured", c.Kind))
	}

	verifier, err := s.lookupVerifier(c.Verifier)
	if err != nil {
		return refuse("verifier", err.Error())
	}
	if c.Kind == objective.KindVerified {
		if err := refuseSamples(field, c); err != nil {
			return err
		}
		if err := refuseBaseline(field, c); err != nil {
			return err
		}
		// A pass or a fail declares no parameters to take.
		return refuseParams(field, c.Params, nil)
	}

	if c.Metric == "" {
		return refuse("metric", "a measured criterion names the metric it compares")
	}
	metric, ok := verifier.DeclaredMetric(c.Metric)
	if !ok {
		return refuse("metric", fmt.Sprintf("verifier %q declares no metric %q", c.Verifier, c.Metric))
	}
	if c.Unit != metric.Unit {
		return refuse("unit", fmt.Sprintf("metric %q is measured in %q, not %q", c.Metric, metric.Unit, c.Unit))
	}
	switch c.Op {
	case objective.OpLess, objective.OpLessEqual, objective.OpGreater, objective.OpGreaterEqual, objective.OpEqual:
	default:
		return refuse("op", fmt.Sprintf("op %q is not one of <, <=, >, >= or ==", c.Op))
	}
	if c.Target == nil {
		return refuse("target", fmt.Sprintf("op %s needs a target to compare with", c.Op))
	}
	if err := validateSampling(field, c); err != nil {
		return err
	}
	if err := validateBaseline(field, c); err != nil {
		return err
	}
	return refuseParams(field, c.Params, metric.Params)
}

// refuseBaseline refuses a baseline or a change on a criterion that is not
// measured: only a measured criterion has a value to set against a baseline.
func refuseBaseline(field string, c objective.Criterion) error {
	var name string
	switch {
	case c.Baseline != nil:
		name = "baseline"
	case c.Change != "":
		name = "change"
	default:
		return nil
	}
	return &ValidationError{Field: field + "." + name, Err: fmt.Errorf("only a measured criterion is set against a baseline, so %s is refused on a %s one", name, c.Kind)}
}

// validateBaseline refuses a measured criterion's baseline and change when
// the loop could never compare with them: a change with no baseline to be set
// against, a change it does not know, or a baseline no change reads.
func validateBaseline(field string, c objective.Criterion) error {
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + "." + name, Err: errors.New(why)}
	}
	switch c.Change {
	case "":
		if c.Baseline != nil {
			return refuse("change", "a baseline is read only through a change; say absolute or percent")
		}
		return nil
	case objective.ChangeAbsolute, objective.ChangePercent:
	default:
		return refuse("change", fmt.Sprintf("change %q is not absolute or percent", c.Change))
	}
	if c.Baseline == nil {
		return refuse("baseline", fmt.Sprintf("a %s change needs a baseline, at_start or a number, to be set against", c.Change))
	}
	return nil
}

// freezeBaselines sets each criterion's frozen baseline from what was given:
// a number is the baseline at once, and at_start has none until the start is
// measured. Whatever baseline_value a request sent is discarded, so nobody can
// write the number a relative target is compared with except by giving it as
// the baseline.
func freezeBaselines(criteria []objective.Criterion) {
	for i := range criteria {
		c := &criteria[i]
		c.BaselineValue = nil
		if c.Baseline != nil && !c.Baseline.AtStart {
			v := c.Baseline.Value
			c.BaselineValue = &v
		}
	}
}

// refuseSamples refuses samples, a reducer, k or min_samples on a criterion
// that is not measured: only a measured criterion has values to reduce.
func refuseSamples(field string, c objective.Criterion) error {
	var name string
	switch {
	case c.Samples != 0:
		name = "samples"
	case c.Reducer != "":
		name = "reducer"
	case c.K != 0:
		name = "k"
	case c.MinSamples != 0:
		name = "min_samples"
	default:
		return nil
	}
	return &ValidationError{Field: field + "." + name, Err: fmt.Errorf("only a measured criterion takes samples, so %s is refused on a %s one", name, c.Kind)}
}

// validateSampling refuses a measured criterion's samples, reducer, k or
// min_samples when the loop could never settle them the way the author meant.
// A samples of 0 says how many runs nothing, so k and min_samples are then
// bounded only from below.
func validateSampling(field string, c objective.Criterion) error {
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + "." + name, Err: errors.New(why)}
	}
	if c.Samples < 0 {
		return refuse("samples", fmt.Sprintf("samples %d is negative", c.Samples))
	}
	switch c.Reducer {
	case "", objective.ReducerAll, objective.ReducerKOfN, objective.ReducerMedian,
		objective.ReducerP95, objective.ReducerMin, objective.ReducerMax, objective.ReducerMean:
	case "any":
		return refuse("reducer", "there is no any reducer: rerunning until one run passes hides real bugs; use all or k_of_n")
	default:
		return refuse("reducer", fmt.Sprintf("reducer %q is not one of all, k_of_n, median, p95, min, max or mean", c.Reducer))
	}
	switch {
	case c.Reducer == objective.ReducerKOfN && c.K < 1:
		return refuse("k", "k_of_n needs a k of at least 1")
	case c.Reducer == objective.ReducerKOfN && c.Samples > 0 && c.K > c.Samples:
		return refuse("k", fmt.Sprintf("k %d is more than the %d samples taken", c.K, c.Samples))
	case c.Reducer != objective.ReducerKOfN && c.K != 0:
		return refuse("k", "k is read only by the k_of_n reducer")
	}
	if c.MinSamples < 0 {
		return refuse("min_samples", fmt.Sprintf("min_samples %d is negative", c.MinSamples))
	}
	if c.Samples > 0 && c.MinSamples > c.Samples {
		return refuse("min_samples", fmt.Sprintf("min_samples %d is more than the %d samples taken, so it could never settle", c.MinSamples, c.Samples))
	}
	return nil
}

// validateSustain refuses a sustain reconcile could never read the way its
// author meant: a window it cannot parse or that holds no time, a budget that
// is not a share from 0 to 1, or a negative min_passes. Any kind of criterion
// may be sustained, since each pass settles it met, not_met or inconclusive.
func validateSustain(field string, s *objective.Sustain) error {
	if s == nil {
		return nil
	}
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + ".sustain." + name, Err: errors.New(why)}
	}
	if _, err := objective.ParseWindow(s.Window); err != nil {
		return refuse("window", err.Error())
	}
	if math.IsNaN(s.Budget) || s.Budget < 0 || s.Budget > 1 {
		return refuse("budget", fmt.Sprintf("budget %v is not a share from 0 to 1", s.Budget))
	}
	if s.MinPasses < 0 {
		return refuse("min_passes", fmt.Sprintf("min_passes %d is negative", s.MinPasses))
	}
	return nil
}

// tightensSustain refuses a request sustain that loosens the template's: none
// where the template had one, a larger budget, which forgives more failures, a
// shorter window, which forgets them sooner, or a lower min_passes, which says
// sustained from fewer passes. A sustain where the template had none asks more
// of the criterion, so it needs only to be valid.
func tightensSustain(field string, tmpl, req *objective.Sustain) error {
	if err := validateSustain(field, req); err != nil {
		return err
	}
	if tmpl == nil {
		return nil
	}
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + "." + name, Err: errors.New(why)}
	}
	if req == nil {
		return refuse("sustain", "the template's criterion is held over a window; a request may not remove its sustain")
	}
	tmplWindow, err := objective.ParseWindow(tmpl.Window)
	if err != nil {
		return refuse("sustain.window", "the template's sustain window cannot be read, so no request window can be shown to be at least as long")
	}
	reqWindow, _ := objective.ParseWindow(req.Window) // validated above
	switch {
	case req.Budget > tmpl.Budget:
		return refuse("sustain.budget", fmt.Sprintf("budget %v loosens the template's %v", req.Budget, tmpl.Budget))
	case reqWindow < tmplWindow:
		return refuse("sustain.window", fmt.Sprintf("window %s is shorter than the template's %s", req.Window, tmpl.Window))
	case req.MinPasses < tmpl.MinPasses:
		return refuse("sustain.min_passes", fmt.Sprintf("min_passes %d is lower than the template's %d", req.MinPasses, tmpl.MinPasses))
	}
	return nil
}

// lookupVerifier returns the pack verifier an ID names. The reserved namespace
// is refused before the lookup, so a discovered tool and one that merely might
// be discovered later are refused alike (ADR 022).
func (s *Service) lookupVerifier(id capability.CapabilityID) (capability.Capability, error) {
	if id == "" {
		return capability.Capability{}, errors.New("a verified or measured criterion names its verifier")
	}
	if capability.IsMCPCapability(id) {
		return capability.Capability{}, fmt.Errorf("verifier %q is in the reserved namespace of discovered tools, which never settle a criterion", id)
	}
	if s.verifiers == nil {
		// Blind, not quiet: without a lookup nothing can say the verifier
		// exists, so the criterion is refused rather than accepted unchecked.
		return capability.Capability{}, fmt.Errorf("verifier %q cannot be checked: no verifier lookup is configured", id)
	}
	v, ok := s.verifiers.Get(id)
	if !ok {
		return capability.Capability{}, fmt.Errorf("no verifier %q is registered", id)
	}
	return v, nil
}

// refuseParams names the first parameter, in name order, that the metric does
// not declare.
func refuseParams(field string, params map[string]any, declared []capability.MetricParam) error {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !slices.ContainsFunc(declared, func(p capability.MetricParam) bool { return p.Name == name }) {
			return &ValidationError{Field: field + ".params." + name, Err: errors.New("the metric declares no such parameter")}
		}
	}
	return nil
}

// tightens refuses a request criterion that does anything to a template
// criterion other than make it stricter: a target further in its op's
// direction (against its baseline, when it has one; see tightensTarget), a
// sustain at least as strict (see tightensSustain), or required where the
// template had it optional. Everything that
// says what is measured, and by whom, stays the template's, and the prose may
// be reworded but not dropped.
func tightens(field string, tmpl, req objective.Criterion) error {
	refuse := func(name, why string) error {
		return &ValidationError{Field: field + "." + name, Err: errors.New(why)}
	}
	switch {
	case req.Description == "":
		return refuse("description", "a criterion needs a description a person can read")
	case req.Kind != tmpl.Kind:
		return refuse("kind", fmt.Sprintf("the template's criterion is %q; a request may tighten it, not change its kind", tmpl.Kind))
	case req.Verifier != tmpl.Verifier:
		return refuse("verifier", fmt.Sprintf("the template's criterion is settled by %q; a request may not change its verifier", tmpl.Verifier))
	case req.Domain != tmpl.Domain:
		return refuse("domain", "a request may not change the domain a template criterion's verifier is resolved in")
	case req.Metric != tmpl.Metric:
		return refuse("metric", fmt.Sprintf("the template's criterion measures %q; a request may not change its metric", tmpl.Metric))
	case req.Unit != tmpl.Unit:
		return refuse("unit", fmt.Sprintf("the template's criterion is in %q; a request may not change its unit", tmpl.Unit))
	case !reflect.DeepEqual(req.Params, tmpl.Params):
		return refuse("params", "a request may not change what a template criterion is measured over")
	case req.Samples != tmpl.Samples || req.Reducer != tmpl.Reducer || req.K != tmpl.K || req.MinSamples != tmpl.MinSamples:
		// A min reducer, a lower k or fewer samples loosens a criterion as
		// surely as a lower target does.
		return refuse("reducer", "a request may not change how a template criterion's samples are taken or reduced")
	case req.Op != tmpl.Op:
		return refuse("op", fmt.Sprintf("the template's criterion compares with %s; a request may not change its op", tmpl.Op))
	case tmpl.Target != nil && req.Target == nil:
		return refuse("target", fmt.Sprintf("the template's criterion has target %v; a request may not remove it", *tmpl.Target))
	}
	if err := tightensTarget(field, tmpl, req); err != nil {
		return err
	}
	if err := tightensSustain(field, tmpl.Sustain, req.Sustain); err != nil {
		return err
	}
	switch {
	case tmpl.IsRequired() && !req.IsRequired():
		return refuse("required", "the template's criterion is required; a request may not make it optional")
	case req.Kind == objective.KindJudged && req.Evidence == "":
		return refuse("evidence", "a judged criterion must name what the judge will look at")
	}
	return nil
}

// tightensTarget refuses a request target that loosens the template's. With
// the same baseline and change the targets compare directly. A changed
// baseline or change is a loosening unless it is plainly stricter: both sides
// must come down to a raw value the measurement is compared with, and the
// request's must be at least as hard to meet. When either cannot, as with
// at_start, whose number nobody knows yet, it is refused, naming the field.
func tightensTarget(field string, tmpl, req objective.Criterion) error {
	if req.Change == tmpl.Change && sameBaseline(req.Baseline, tmpl.Baseline) {
		if tmpl.Target != nil && !stricterOrEqual(req.Op, *req.Target, *tmpl.Target) {
			return &ValidationError{Field: field + ".target", Err: fmt.Errorf("target %v loosens the template's %v for op %s", *req.Target, *tmpl.Target, tmpl.Op)}
		}
		return nil
	}
	name := "change"
	if req.Change == tmpl.Change {
		name = "baseline"
	}
	reqRaw, reqOK := rawThreshold(req)
	tmplRaw, tmplOK := rawThreshold(tmpl)
	if !reqOK || !tmplOK {
		return &ValidationError{Field: field + "." + name, Err: errors.New("a request may change a template criterion's baseline or change only where it is plainly stricter, and this one cannot be compared with the template's")}
	}
	if !stricterOrEqual(tmpl.Op, reqRaw, tmplRaw) {
		return &ValidationError{Field: field + "." + name, Err: fmt.Errorf("against its baseline the target comes to %v, which loosens the template's %v for op %s", reqRaw, tmplRaw, tmpl.Op)}
	}
	return nil
}

func sameBaseline(a, b *objective.Baseline) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// rawThreshold is the raw value a measured criterion's target comes to: the
// target itself with no change, and against a number baseline the value the
// measurement must reach for the change to meet it, since value-baseline op t
// is value op baseline+t, and a percent change is value op
// baseline+t*|baseline|/100. An at_start baseline, a percent change against
// 0, a change with no baseline or a baseline with no change comes to nothing
// known, so it reports false.
func rawThreshold(c objective.Criterion) (float64, bool) {
	if c.Target == nil {
		return 0, false
	}
	t := *c.Target
	if c.Change == "" {
		return t, c.Baseline == nil
	}
	if c.Baseline == nil || c.Baseline.AtStart {
		return 0, false
	}
	b := c.Baseline.Value
	switch c.Change {
	case objective.ChangeAbsolute:
		return b + t, true
	case objective.ChangePercent:
		if b == 0 {
			return 0, false
		}
		return b + t*math.Abs(b)/100, true
	}
	return 0, false
}

// stricterOrEqual reports whether target is at least as hard to meet as the
// template's for op.
func stricterOrEqual(op objective.Op, target, tmpl float64) bool {
	switch op {
	case objective.OpGreater, objective.OpGreaterEqual:
		return target >= tmpl
	case objective.OpLess, objective.OpLessEqual:
		return target <= tmpl
	default:
		return target == tmpl
	}
}

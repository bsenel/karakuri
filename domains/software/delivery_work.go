package software

// Whether delivery has work (Phase 40 step 13), computed without a model.
//
// The delivery stream's reconcile fingerprints this environment's snapshot, so
// the supervisor's existing drift check starts a pass when the verdict moves
// and makes no model call while it does not.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// DeliveryWork is the verdict: whether a delivery pass has anything to do and,
// in words, why or why not.
type DeliveryWork struct {
	HasWork bool
	// Unmergeable is the delivery stream's open pull requests that cannot be
	// merged, by branch, sorted (Phase 40 step 14a). It is not work and moves
	// neither HasWork, Reasons nor the snapshot hash.
	Unmergeable []string
	Reasons     []string
}

var (
	plannedPhaseHeading = regexp.MustCompile(`^## Phase (\d+(?:\.\d+)?)\b.*\(Planned\)\s*$`)
	backlogID           = regexp.MustCompile(`^EB-\d+$`)
	prTitlePhase        = regexp.MustCompile(`(?i)\bphase (\d+(?:\.\d+)?)\b`)
	prTitleBacklogID    = regexp.MustCompile(`\bEB-\d+\b`)
)

// plannedItems lists the roadmap's Planned phases ("phase 40") and Engineering
// Backlog entries whose Status is Planned ("EB-014"), in file order.
func plannedItems(roadmap string) []string {
	var items []string
	for _, line := range strings.Split(roadmap, "\n") {
		if m := plannedPhaseHeading.FindStringSubmatch(line); m != nil {
			items = append(items, "phase "+m[1])
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 2 {
			continue
		}
		id := strings.TrimSpace(cells[0])
		if backlogID.MatchString(id) && strings.TrimSpace(cells[len(cells)-1]) == "Planned" {
			items = append(items, id)
		}
	}
	return items
}

// ComputeDeliveryWork reads main's roadmap text, the open pull requests and the
// git observation's state (branches_without_pr, unanswered_review_comments).
// A roadmap that could not be read is an error, never "no work" (AGENTS.md rule 10).
func ComputeDeliveryWork(roadmap string, roadmapErr error, prs []versioncontrol.PRSummary, state map[string]any) (DeliveryWork, error) {
	if roadmapErr != nil {
		return DeliveryWork{}, fmt.Errorf("main's roadmap could not be read: %w", roadmapErr)
	}
	// The findings come from the forge too; one that could not be read is
	// blind as well, since an empty list would read as nothing waiting.
	for _, key := range []string{"branches_without_pr_error", "unanswered_review_comments_error", "unmergeable_prs_error"} {
		if msg, ok := state[key]; ok {
			// A forge that has no such signal has no such work; only a read
			// that failed is blind.
			if strings.Contains(fmt.Sprint(msg), versioncontrol.ErrUnsupported.Error()) {
				continue
			}
			return DeliveryWork{}, fmt.Errorf("%s: %v", key, msg)
		}
	}
	covered := map[string]int{} // item -> number of the open pull request that covers it
	for _, pr := range prs {
		for _, m := range prTitlePhase.FindAllStringSubmatch(pr.Title, -1) {
			covered["phase "+m[1]] = pr.Number
		}
		for _, id := range prTitleBacklogID.FindAllString(pr.Title, -1) {
			covered[id] = pr.Number
		}
	}
	var w DeliveryWork
	for _, item := range plannedItems(roadmap) {
		if n, ok := covered[item]; ok {
			w.Reasons = append(w.Reasons, fmt.Sprintf("%s is Planned and pull request %d is open, waiting for the owner", item, n))
			continue
		}
		w.HasWork = true
		w.Reasons = append(w.Reasons, item+" is Planned on main with no open pull request")
	}
	branches, _ := state["branches_without_pr"].([]map[string]any)
	for _, b := range branches {
		w.HasWork = true
		w.Reasons = append(w.Reasons, fmt.Sprintf("branch %v has commits and no pull request", b["branch"]))
	}
	if comments, _ := state["unanswered_review_comments"].([]map[string]any); len(comments) > 0 {
		w.HasWork = true
		w.Reasons = append(w.Reasons, fmt.Sprintf("%d unanswered review comment(s) on stream pull requests", len(comments)))
	}
	// Shown, not work: the delivery stream's own pull requests only.
	unmergeable, _ := state["unmergeable_prs"].([]map[string]any)
	for _, u := range unmergeable {
		if u["stream"] == roadmapDeliveryTemplateID {
			w.Unmergeable = append(w.Unmergeable, fmt.Sprint(u["branch"]))
		}
	}
	sort.Strings(w.Unmergeable)
	sort.Strings(w.Reasons)
	return w, nil
}

// NewDeliveryWorkEnvironment is the environment the delivery stream's
// reconcile fingerprints: its snapshot is a hash of the computed verdict and
// what it was computed from, so it moves when the work changes and not otherwise.
//
// ownAuthors and posted are what the git observation gets (Phase 40 step 12a):
// whose comments are the deployment's own, and the record of what Karakuri
// posted from a shared account; a nil posted means no record.
func NewDeliveryWorkEnvironment(id environment.EnvironmentID, vc versioncontrol.VersionControlAdapter, readMain func(context.Context) (string, error), ownAuthors config.OwnAuthorsConfig, posted func(context.Context) (map[int64]bool, error)) environment.Environment {
	return &deliveryWorkEnv{id: id, vc: vc, readMain: readMain, ownAuthors: ownAuthors, posted: posted}
}

type deliveryWorkEnv struct {
	id         environment.EnvironmentID
	vc         versioncontrol.VersionControlAdapter
	readMain   func(context.Context) (string, error)
	ownAuthors config.OwnAuthorsConfig
	posted     postedCommentIDs
}

func (e *deliveryWorkEnv) ID() environment.EnvironmentID { return e.id }
func (e *deliveryWorkEnv) Domain() string                { return "software" }

// Observe computes the verdict. Anything it could not read is an error: blind
// must never look like no work.
func (e *deliveryWorkEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	if e.vc == nil || e.readMain == nil {
		return environment.Observation{}, errors.New("no forge or no source for main's roadmap is bound")
	}
	work, _, err := readDeliveryWork(ctx, e.vc, e.readMain, e.ownAuthors, e.posted)
	if err != nil {
		return environment.Observation{}, err
	}
	// Only the verdict and its reasons are exposed: no pull request title or
	// comment body, so the observation carries nothing a third party wrote.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v|%s", work.HasWork, strings.Join(work.Reasons, "\n"))))
	return environment.Observation{
		EnvID: e.id, State: map[string]any{"has_work": work.HasWork, "reasons": work.Reasons, "unmergeable_prs": work.Unmergeable},
		Version: hex.EncodeToString(sum[:])[:16], Timestamp: time.Now().UTC(), Trust: environment.TrustOperator,
	}, nil
}

// readDeliveryWork reads main's roadmap, the open pull requests and the git
// observation's state, and computes the verdict; the discovery trigger reads
// the same thing, so the two never disagree about what is in the pipeline.
func readDeliveryWork(ctx context.Context, vc versioncontrol.VersionControlAdapter, readMain func(context.Context) (string, error), authors config.OwnAuthorsConfig, posted postedCommentIDs) (DeliveryWork, []versioncontrol.PRSummary, error) {
	roadmap, roadmapErr := readMain(ctx)
	prs, err := vc.ListPRs(ctx, "", time.Time{})
	if err != nil {
		return DeliveryWork{}, nil, fmt.Errorf("open pull requests could not be read: %w", err)
	}
	state := map[string]any{}
	observeBranchesWithoutPR(ctx, vc, "", state)
	observeUnansweredReviewComments(ctx, vc, "", authors, 0, posted, state)
	observeUnmergeablePRs(ctx, vc, "", state)
	work, err := ComputeDeliveryWork(roadmap, roadmapErr, prs, state)
	return work, prs, err
}

func (e *deliveryWorkEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf("%s: the delivery-work environment only observes", a.CapabilityID)}, nil
}

func (e *deliveryWorkEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

// Snapshot has an empty SHA when Observe could not read; the supervisor treats
// that as blind.
func (e *deliveryWorkEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, err := e.Observe(ctx, environment.ObservationQuery{})
	if err != nil {
		// Blind is an empty SHA, not an error (AGENTS.md rule 10).
		return environment.EnvironmentSnapshot{EnvID: e.id}, nil
	}
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

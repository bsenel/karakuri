package report

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/objective"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/schedule"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Config bounds the sender.
type Config struct {
	// Enabled turns delivery off without deleting anybody's schedules.
	Enabled bool
	Tick    time.Duration
	// LeaseTTL is how long a claim on a schedule survives. Short, because a
	// digest takes seconds and a stuck lease delays a report somebody is
	// waiting for.
	LeaseTTL time.Duration
}

func (c Config) withDefaults() Config {
	if c.Tick <= 0 {
		c.Tick = time.Minute
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 2 * time.Minute
	}
	return c
}

type Service struct {
	store   storage.StorageAdapter
	tools   *tools.Registry
	factory *platformagent.Factory
	quota   karakuriquota.Deps
	cfg     Config
	holder  string
	now     func() time.Time
}

func NewService(
	store storage.StorageAdapter,
	toolReg *tools.Registry,
	factory *platformagent.Factory,
	quotaDeps karakuriquota.Deps,
	cfg Config,
) *Service {
	return &Service{
		store:   store,
		tools:   toolReg,
		factory: factory,
		quota:   quotaDeps,
		cfg:     cfg.withDefaults(),
		holder:  newHolderID(),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Start runs the sender on its own ticker.
//
// A minute rather than the supervisor's thirty seconds: a digest is scheduled
// in hours or days, and a report that goes out within a minute of its time is
// on time by any standard a reader applies.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		slog.Info("digest sender disabled by configuration")
		return
	}
	slog.Info("digest sender started", "holder", s.holder, "tick", s.cfg.Tick.String())
	go func() {
		ticker := time.NewTicker(s.cfg.Tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Tick(ctx)
			}
		}
	}()
}

// Tick sends whatever is due. Exported so tests drive it directly.
func (s *Service) Tick(ctx context.Context) {
	now := s.now()
	due, err := s.store.ListDueReportSchedules(ctx, s.holder, now, 25)
	if err != nil {
		slog.Warn("digest due query failed", "err", err)
		return
	}
	for _, sch := range due {
		if err := s.Send(ctx, sch.ID); err != nil {
			slog.Warn("digest delivery failed", "schedule", sch.ID, "err", err)
		}
	}
}

// Declare creates or updates a schedule and arms it.
func (s *Service) Declare(ctx context.Context, sch digest.Schedule) (digest.Schedule, error) {
	if sch.TwinID == "" {
		return digest.Schedule{}, fmt.Errorf("a report schedule needs a twin")
	}
	if !digest.ValidChannel(sch.Channel) {
		return digest.Schedule{}, fmt.Errorf("unknown channel %q", sch.Channel)
	}
	if err := schedule.Validate(sch.Cadence); err != nil {
		return digest.Schedule{}, err
	}
	if sch.Window != "" && sch.WindowDuration() == 0 {
		return digest.Schedule{}, fmt.Errorf("window %q is not a positive duration", sch.Window)
	}
	if sch.ID == "" {
		sch.ID = "rep_" + newID()
		sch.CreatedAt = s.now()
	}
	sch.Enabled = true

	// Armed for the cadence's next firing rather than for now.
	//
	// The scheduler treats a never-run thing as due immediately, which is
	// right for an objective — somebody has just declared a state and the
	// first useful answer is whether it already holds. It is wrong for a
	// report: a digest declared at 3pm for an 8am schedule would fire at once,
	// covering a window nobody asked about, and the reader's first experience
	// of their morning brief would be an off-schedule one. So the declaration
	// itself counts as the last firing.
	now := s.now()
	plan, err := schedule.Next(sch.Cadence, schedule.Reference{Now: now, LastReconciledAt: now})
	if err != nil {
		return digest.Schedule{}, err
	}
	sch.NextDueAt = nextOrNil(plan.Reconcile, now, sch.Cadence)

	if err := s.store.SaveReportSchedule(ctx, sch); err != nil {
		return digest.Schedule{}, err
	}
	return sch, nil
}

func (s *Service) Get(ctx context.Context, id string) (digest.Schedule, error) {
	return s.store.GetReportSchedule(ctx, id)
}

func (s *Service) List(ctx context.Context, twinID string) ([]digest.Schedule, error) {
	return s.store.ListReportSchedules(ctx, twinID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteReportSchedule(ctx, id)
}

// Preview assembles and renders a digest without sending it, so somebody can
// see what a schedule will produce before committing a daily mail to it.
func (s *Service) Preview(ctx context.Context, twinID string, window time.Duration) (digest.Digest, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	now := s.now()
	d, err := s.Assemble(ctx, twinID, now.Add(-window), now)
	if err != nil {
		return d, err
	}
	d.Prose = s.render(ctx, d)
	return d, nil
}

// Send assembles, renders and delivers one schedule's digest, then re-arms it.
func (s *Service) Send(ctx context.Context, id string) error {
	sch, err := s.store.GetReportSchedule(ctx, id)
	if err != nil {
		return err
	}
	now := s.now()

	claimed, err := s.store.ClaimReportSchedule(ctx, id, s.holder, now, now.Add(s.cfg.LeaseTTL))
	if err != nil {
		return err
	}
	if !claimed {
		// Another replica is sending it. This is the case the lease exists
		// for: two replicas reconciling one objective wastes money, while two
		// replicas sending one morning report send it to a person twice.
		return nil
	}
	defer func() { _ = s.store.ReleaseReportSchedule(context.WithoutCancel(ctx), id, s.holder) }()

	since, until := windowOf(sch, now)
	d, err := s.Assemble(ctx, sch.TwinID, since, until)
	if err != nil {
		return s.rearm(ctx, sch, err)
	}

	if d.Empty() && !sch.SendWhenEmpty {
		// Nothing happened. Re-arm and say nothing: a daily mail that says
		// "nothing happened" is a mail people stop reading, which costs the
		// ones that matter their audience.
		//
		// LastSentAt still moves, so the next window starts here rather than
		// accumulating a week of silence into one report.
		sch.LastSentAt = &until
		return s.rearm(ctx, sch, nil)
	}

	d.Prose = s.render(ctx, d)
	if err := s.deliver(ctx, sch, d); err != nil {
		return s.rearm(ctx, sch, err)
	}
	sch.LastSentAt = &until
	return s.rearm(ctx, sch, nil)
}

// rearm records the outcome and computes the next firing. It is the single
// exit from a send, so no path can leave a schedule unarmed.
func (s *Service) rearm(ctx context.Context, sch digest.Schedule, sendErr error) error {
	sch.LastError = ""
	if sendErr != nil {
		sch.LastError = sendErr.Error()
	}
	now := s.now()
	if plan, err := schedule.Next(sch.Cadence, s.reference(sch)); err == nil {
		sch.NextDueAt = nextOrNil(plan.Reconcile, now, sch.Cadence)
	} else {
		// A cadence that will not parse should not silently stop the report.
		// Retry in an hour and leave the error where somebody will see it.
		next := now.Add(time.Hour)
		sch.NextDueAt = &next
	}
	if err := s.store.SaveReportSchedule(ctx, sch); err != nil {
		return err
	}
	return sendErr
}

func (s *Service) reference(sch digest.Schedule) schedule.Reference {
	ref := schedule.Reference{Now: s.now()}
	if sch.LastSentAt != nil {
		ref.LastReconciledAt = *sch.LastSentAt
	}
	return ref
}

// nextOrNil guards against a schedule that never fires. A cadence with no
// reconcile half — somebody declared only a timezone — would otherwise arm to
// nil and go quiet forever without saying so; an hourly fallback keeps it
// visible.
func nextOrNil(next, now time.Time, cadence objective.Cadence) *time.Time {
	if !next.IsZero() {
		u := next.UTC()
		return &u
	}
	if cadence.HasSchedule() || cadence.ReconcileInterval() > 0 {
		return nil
	}
	fallback := now.Add(time.Hour)
	return &fallback
}

// costs reads the ledger for one twin's window, grouped by provider.
func (s *Service) costs(ctx context.Context, twinID string, since, until time.Time) ([]cost.Bucket, error) {
	return s.quota.CostReport(ctx, cost.Query{
		Since:    since,
		Until:    until,
		Subjects: []quota.Key{karakuriquota.CostSubject(twinID)},
		GroupBy:  []cost.GroupBy{cost.ByProvider},
	})
}

func decodeAutonomyPayload(raw string) (from, to objective.AutonomyLevel, reason string) {
	var p struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Reason string `json:"reason"`
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	return objective.AutonomyLevel(p.From), objective.AutonomyLevel(p.To), p.Reason
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newHolderID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b))
}

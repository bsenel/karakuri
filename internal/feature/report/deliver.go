package report

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/email"
)

// deliver sends the digest through the twin's bound adapter.
//
// It goes through the same tool registry the loop's act step uses, resolved by
// the twin's adapter bindings (ADR 006), so a multi-tenant deployment reaches
// the right Slack workspace without this code knowing tenants exist. And it
// writes its own `tool_events` row, because a message Karakuri sent on
// somebody's behalf is a thing it did to the world, and the audit log is where
// those live — a delivery invisible to `krk audit` would be the one kind of
// action nobody could review.
func (s *Service) deliver(ctx context.Context, sch digest.Schedule, d digest.Digest) error {
	instance := s.resolveInstance(ctx, sch)
	body := d.Prose
	if body == "" {
		body = Plain(d)
	}

	var err error
	switch sch.Channel {
	case digest.ChannelMessaging:
		err = s.deliverMessaging(ctx, instance, sch.Target, body)
	case digest.ChannelEmail:
		err = s.deliverEmail(ctx, instance, sch.Target, subject(d), body)
	default:
		// projectmgmt and versioncontrol are declared as channels and not yet
		// wired. An honest refusal rather than a silent success: the schedule
		// records the error and an operator sees why nothing arrived.
		err = fmt.Errorf("channel %q cannot deliver yet", sch.Channel)
	}

	s.audit(ctx, sch, d, err)
	return err
}

func (s *Service) deliverMessaging(ctx context.Context, instance, target, body string) error {
	if s.tools == nil {
		return fmt.Errorf("no tool registry configured")
	}
	adapter, ok := s.tools.Messaging.Resolve(instance)
	if !ok || !adapter.Active() {
		return fmt.Errorf("messaging adapter %q is not configured", instance)
	}
	if target == "" {
		return fmt.Errorf("messaging delivery needs a target channel")
	}
	return adapter.PostMessage(ctx, target, body)
}

func (s *Service) deliverEmail(ctx context.Context, instance, target, subj, body string) error {
	if s.tools == nil {
		return fmt.Errorf("no tool registry configured")
	}
	adapter, ok := s.tools.Email.Resolve(instance)
	if !ok || !adapter.Active() {
		return fmt.Errorf("email adapter %q is not configured", instance)
	}
	if target == "" {
		return fmt.Errorf("email delivery needs a recipient")
	}
	_, err := adapter.Send(ctx, email.Message{
		To:      []string{target},
		Subject: subj,
		Body:    body,
	})
	return err
}

// resolveInstance prefers what the schedule named, then what the twin is bound
// to, then the slot default. The schedule can name one because a twin might
// report to a private channel through one workspace while working through
// another.
func (s *Service) resolveInstance(ctx context.Context, sch digest.Schedule) string {
	if sch.Instance != "" {
		return sch.Instance
	}
	if t, err := s.store.GetTwin(ctx, sch.TwinID); err == nil {
		if name, ok := t.AdapterBindings[sch.Channel]; ok {
			return name
		}
	}
	return ""
}

// subject leads with the count of decisions when there are any.
//
// A subject line is the only part of a mail that is always read, so it carries
// the one fact that decides whether the mail is opened now or later.
func subject(d digest.Digest) string {
	name := d.TwinName
	if name == "" {
		name = d.TwinID
	}
	date := d.Until.Format("2 Jan")
	if n := len(d.Decisions); n > 0 {
		return fmt.Sprintf("[%d to decide] %s — %s", n, name, date)
	}
	return fmt.Sprintf("%s — %s", name, date)
}

// audit records the delivery, successful or not, in the same log the loop's
// actions and the checkpoint approvals share.
func (s *Service) audit(ctx context.Context, sch digest.Schedule, d digest.Digest, deliveryErr error) {
	payload := map[string]any{
		"schedule_id": sch.ID,
		"twin_id":     sch.TwinID,
		"channel":     sch.Channel,
		"target":      sch.Target,
		"since":       d.Since,
		"until":       d.Until,
		"decisions":   len(d.Decisions),
		"objectives":  len(d.Objectives),
	}
	if deliveryErr != nil {
		payload["error"] = deliveryErr.Error()
	}
	pj, _ := json.Marshal(payload)

	_ = s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		AgentID:     "karakuri-reporter",
		Capability:  "deliver.digest",
		Adapter:     sch.Channel,
		Kind:        storage.ToolEventExecute,
		Success:     deliveryErr == nil,
		PayloadJSON: string(pj),
	})
}

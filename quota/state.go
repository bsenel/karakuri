package quota

import "time"

// Event is one recorded consumption, used by [SlidingLog]. Costs are folded
// into N rather than repeated, so a single expensive call does not store a
// timestamp per unit.
type Event struct {
	At time.Time
	N  int
}

// State is everything a backend has to remember about one key.
//
// It is the persistence contract: a backend loads it, calls [Apply], and stores
// what comes back. Which fields matter depends on the algorithm — a token
// bucket uses Tokens and Last, the windows use the others — and a backend is
// free to store only those, though storing the union keeps a take to one row.
type State struct {
	// Algorithm the stored fields belong to. [Apply] resets the state when it
	// does not match the policy, because the field shapes are not
	// interchangeable and reading a token count as a window counter would be a
	// silent wrong answer rather than a loud one.
	Algorithm Algorithm

	// TokenBucket.
	Tokens float64
	Last   time.Time

	// FixedWindow.
	WindowStart time.Time
	Count       int

	// SlidingLog, oldest first. Apply trims expired entries, so what it leaves
	// behind is what should be stored.
	Log []Event
}

// Apply runs p against s for a cost of n at now, mutating s and returning the
// decision.
//
// This is the arithmetic every backend shares. It lives here rather than in
// each of them because a second implementation of a token bucket is a second
// chance to get the refill wrong, and the difference would show up as a limit
// that behaves differently depending on which backend is configured — which is
// exactly the class of bug nobody reproduces.
//
// Backends remain responsible for atomicity: Apply assumes exclusive access to
// s for the duration of the call.
func Apply(s *State, p Policy, n int, now time.Time) (Decision, error) {
	if err := p.Validate(); err != nil {
		return Decision{}, err
	}
	if n < 0 {
		n = 0
	}
	if s.Algorithm != p.Algorithm {
		*s = State{Algorithm: p.Algorithm}
	}

	var d Decision
	switch p.Algorithm {
	case TokenBucket:
		d = applyTokenBucket(s, p, n, now)
	case FixedWindow:
		d = applyFixedWindow(s, p, n, now)
	case SlidingLog:
		d = applySlidingLog(s, p, n, now)
	}
	return d.Normalize(), nil
}

func applyTokenBucket(s *State, p Policy, n int, now time.Time) Decision {
	rate := p.refillRate()
	capacity := float64(p.Limit)

	if s.Last.IsZero() {
		s.Tokens = capacity
		s.Last = now
	}
	// Only ever refill forward. A clock that jumps backwards must not drain the
	// bucket, and must not be recorded as the new baseline either.
	if elapsed := now.Sub(s.Last); elapsed > 0 {
		s.Tokens = min(capacity, s.Tokens+elapsed.Seconds()*rate)
		s.Last = now
	}

	d := Decision{Limit: p.Limit}
	// tokenEpsilon absorbs floating-point rounding. Refilling for exactly the
	// time one token takes can land a hair under it, which would refuse a
	// request that arrived precisely on schedule. A nanotoken of slack costs
	// nothing real and makes the boundary behave the way the arithmetic says it
	// should.
	const tokenEpsilon = 1e-9
	if s.Tokens+tokenEpsilon >= float64(n) {
		s.Tokens -= float64(n)
		d.Allowed = true
	} else {
		d.RetryAfter = seconds((float64(n) - s.Tokens) / rate)
	}
	d.Remaining = int(s.Tokens)
	d.ResetAt = now.Add(seconds((capacity - s.Tokens) / rate))
	return d
}

func applyFixedWindow(s *State, p Policy, n int, now time.Time) Decision {
	// Windows are anchored to the epoch rather than to first use, so every
	// replica and every restart agrees on where the boundary falls. A 24h
	// window lands on midnight UTC because the zero time is midnight UTC.
	start := now.Truncate(p.Window)
	if !s.WindowStart.Equal(start) {
		s.WindowStart, s.Count = start, 0
	}
	end := start.Add(p.Window)

	d := Decision{Limit: p.Limit, ResetAt: end}
	if s.Count+n <= p.Limit {
		s.Count += n
		d.Allowed = true
	} else {
		d.RetryAfter = end.Sub(now)
	}
	d.Remaining = max(p.Limit-s.Count, 0)
	return d
}

func applySlidingLog(s *State, p Policy, n int, now time.Time) Decision {
	cutoff := now.Add(-p.Window)
	kept := s.Log[:0]
	used := 0
	for _, e := range s.Log {
		if e.At.After(cutoff) {
			kept = append(kept, e)
			used += e.N
		}
	}
	s.Log = kept

	d := Decision{Limit: p.Limit, Remaining: max(p.Limit-used, 0)}
	if len(s.Log) > 0 {
		d.ResetAt = s.Log[0].At.Add(p.Window)
	} else {
		d.ResetAt = now
	}

	if used+n <= p.Limit {
		if n > 0 {
			s.Log = append(s.Log, Event{At: now, N: n})
			d.Remaining = max(p.Limit-(used+n), 0)
		}
		d.Allowed = true
		return d
	}

	// Refused: wait until enough of the oldest entries have aged out to make
	// room for n. Walking from the oldest is what makes this the *earliest*
	// time the request would succeed rather than a conservative guess.
	need := used + n - p.Limit
	freed := 0
	for _, e := range s.Log {
		freed += e.N
		if freed >= need {
			d.RetryAfter = e.At.Add(p.Window).Sub(now)
			break
		}
	}
	if d.RetryAfter <= 0 {
		// n alone exceeds the limit, so no amount of waiting helps. Report the
		// full window rather than nothing, which would invite an instant retry.
		d.RetryAfter = p.Window
	}
	return d
}

// seconds converts a float number of seconds into a Duration, clamping the
// negative and absurd cases that arithmetic on a caller-supplied rate can
// produce — a Duration is nanoseconds in an int64, so a large enough float
// silently wraps to a negative wait.
func seconds(s float64) time.Duration {
	if s <= 0 || s != s { // NaN compares false against itself
		return 0
	}
	// A Duration tops out at about 292 years. Clamping in *seconds* has to stay
	// well under that, because the multiply below is what overflows.
	const maxSeconds = float64(1 << 33) // ~272 years
	return time.Duration(min(s, maxSeconds) * float64(time.Second))
}

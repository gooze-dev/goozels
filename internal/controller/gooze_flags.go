package controller

import (
	"context"
	"time"
)

// probeTimeout bounds asking gooze for its flags, so a hung binary cannot
// leave the probe running for the whole session.
//
// Equivalent: the exact bound is a tuning choice; any duration long enough
// for gooze to print its help behaves the same.
const probeTimeout = 10 * time.Second //gooze:ignore arithmetic,numbers

// probeGooze asks the installed gooze, once per session and in the
// background, which run flags it supports. Until it answers, nothing is.
func (s *Server) probeGooze() {
	s.probe.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()

			s.loadGoozeFlags(ctx)
		}()
	})
}

// loadGoozeFlags records gooze's run flags; a failed probe records none.
func (s *Server) loadGoozeFlags(ctx context.Context) {
	flags, err := s.gooze.RunFlags(ctx)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.goozeFlags = flags
	s.mu.Unlock()
}

// goozeSupports reports whether the installed gooze's run command has the
// long flag name (without dashes), as far as the probe has found out.
func (s *Server) goozeSupports(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.goozeFlags[name]
}

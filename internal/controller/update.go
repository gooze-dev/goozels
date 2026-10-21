package controller

import (
	"context"
	"time"

	"gooze.dev/pkg/goozels/internal/domain/update"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// refreshTimeout bounds the background release lookup so a stalled network
// cannot leave the goroutine hanging for the whole session.
const refreshTimeout = 10 * time.Second

// announceUpdate tells the editor about a newer goozels release, at most once
// per session and without delaying any LSP traffic. The cached answer is shown
// right away; a stale cache is refreshed in the background, and its result is
// shown only if it reveals a newer release than the cache did. Lookup failures
// (e.g. offline) are silent.
func (s *Server) announceUpdate() {
	if s.releases == nil || !update.Released(s.version) {
		return
	}

	s.announce.Do(func() {
		cachedTag, fresh := s.releases.Cached()
		s.showUpdate(cachedTag)

		if fresh {
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
			defer cancel()

			latest, err := s.releases.Refresh(ctx)
			if err == nil && latest != cachedTag {
				s.showUpdate(latest)
			}
		}()
	})
}

// showUpdate sends a warning when latest is newer than the running build.
func (s *Server) showUpdate(latest string) {
	if msg, ok := update.Notice(s.version, latest); ok {
		s.notify("window/showMessage", lsp.ShowMessageParams{Type: lsp.MessageWarn, Message: msg})
	}
}

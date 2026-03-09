// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hub

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ptone/scion-agent/pkg/store"
)

// UserActivityTracker records user last-seen timestamps with in-memory
// throttling so that at most one database write occurs per user per
// throttle interval (default: 1 hour).
type UserActivityTracker struct {
	store    store.Store
	interval time.Duration

	mu       sync.Mutex
	lastSeen map[string]time.Time // userID → last flushed time
}

// NewUserActivityTracker creates a tracker that limits last-seen writes
// to at most once per interval per user.
func NewUserActivityTracker(s store.Store, interval time.Duration) *UserActivityTracker {
	return &UserActivityTracker{
		store:    s,
		interval: interval,
		lastSeen: make(map[string]time.Time),
	}
}

// userActivityMiddleware returns middleware that touches the activity
// tracker for each authenticated user request.
func userActivityMiddleware(tracker *UserActivityTracker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if user := GetUserIdentityFromContext(r.Context()); user != nil {
				tracker.Touch(user.ID())
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Touch records activity for a user. If the user was last written more
// than the throttle interval ago (or never), the timestamp is persisted
// asynchronously.
func (t *UserActivityTracker) Touch(userID string) {
	now := time.Now()

	t.mu.Lock()
	last, ok := t.lastSeen[userID]
	if ok && now.Sub(last) < t.interval {
		t.mu.Unlock()
		return
	}
	t.lastSeen[userID] = now
	t.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := t.store.UpdateUserLastSeen(ctx, userID, now); err != nil {
			slog.Warn("Failed to update user last-seen", "userId", userID, "error", err)
		}
	}()
}

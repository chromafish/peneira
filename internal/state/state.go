// Package state holds what you have read and written down while reading a
// change, keyed by jj change ID.
//
// It lives in memory for as long as the window is open and is not written
// anywhere. Notes are written to be copied out to an agent while you read, so
// a session that ends has nothing left to keep.
package state

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Side identifies which half of a diff a comment is anchored to.
type Side string

const (
	SideNew Side = "new"
	SideOld Side = "old"
)

// Comment is a note left on a line of a diff.
type Comment struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Side Side   `json:"side"`
	Line int    `json:"line"`
	// EndLine closes a note written about a run of lines rather than one. It
	// is left out when a note is about a single line, so old state files read
	// back unchanged.
	EndLine   int       `json:"end_line,omitempty"`
	Body      string    `json:"body"`
	CommitID  string    `json:"commit_id"` // the version of the change it was written against
	CreatedAt time.Time `json:"created_at"`
	Resolved  bool      `json:"resolved"`
}

// LastLine is the line a note is filed under: the end of its run, or its only
// line.
func (c Comment) LastLine() int {
	if c.EndLine > c.Line {
		return c.EndLine
	}
	return c.Line
}

// Span reports whether a note covers more than one line.
func (c Comment) Span() bool { return c.EndLine > c.Line }

// FileReview records that a path was marked as read, and at which version.
type FileReview struct {
	ViewedAt string `json:"viewed_at"` // commit ID when marked, "" if not viewed
}

// Change is what has been read and written down about one jj change.
type Change struct {
	ChangeID string
	Files    map[string]*FileReview
	Comments []Comment
}

// Store is the review state for one session. It is safe for concurrent use,
// since background work touches it as well as the frame that draws it.
type Store struct {
	mu      sync.Mutex
	changes map[string]*Change
	seq     int // makes comment IDs unique within a session
}

// New returns an empty store.
func New() *Store {
	return &Store{changes: map[string]*Change{}}
}

func (s *Store) change(changeID string) *Change {
	c := s.changes[changeID]
	if c == nil {
		c = &Change{ChangeID: changeID, Files: map[string]*FileReview{}}
		s.changes[changeID] = c
	}
	return c
}

// Viewed reports whether a path was marked read, and whether that mark is
// stale because the change has been rewritten since.
func (s *Store) Viewed(changeID, path, commitID string) (viewed, stale bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.changes[changeID]
	if c == nil {
		return false, false
	}
	f := c.Files[path]
	if f == nil || f.ViewedAt == "" {
		return false, false
	}
	return true, f.ViewedAt != commitID
}

// SetViewed marks or unmarks a path as read at a given version.
func (s *Store) SetViewed(changeID, path, commitID string, viewed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.change(changeID)
	if viewed {
		c.Files[path] = &FileReview{ViewedAt: commitID}
	} else {
		delete(c.Files, path)
	}
}

// AddComment stores a note against a line and returns it with its ID filled in.
func (s *Store) AddComment(changeID string, c Comment) Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	c.ID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), s.seq)
	c.CreatedAt = time.Now()
	ch := s.change(changeID)
	ch.Comments = append(ch.Comments, c)
	return c
}

// find returns the comment with an ID, or nil. The caller must hold s.mu.
func (s *Store) find(changeID, commentID string) *Comment {
	ch := s.change(changeID)
	for i := range ch.Comments {
		if ch.Comments[i].ID == commentID {
			return &ch.Comments[i]
		}
	}
	return nil
}

// UpdateComment replaces the body of an existing comment.
func (s *Store) UpdateComment(changeID, commentID, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.find(changeID, commentID); c != nil {
		c.Body = body
	}
}

// DeleteComment removes a comment.
func (s *Store) DeleteComment(changeID, commentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.change(changeID)
	ch.Comments = slices.DeleteFunc(ch.Comments, func(c Comment) bool {
		return c.ID == commentID
	})
}

// ClearComments removes every comment on a change and reports how many went.
func (s *Store) ClearComments(changeID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.change(changeID)
	n := len(ch.Comments)
	ch.Comments = nil
	return n
}

// ToggleResolved flips a comment's resolved flag.
func (s *Store) ToggleResolved(changeID, commentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.find(changeID, commentID); c != nil {
		c.Resolved = !c.Resolved
	}
}

// Comments returns the comments on one path, ordered by line.
func (s *Store) Comments(changeID, path string) []Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.changes[changeID]
	if c == nil {
		return nil
	}
	var out []Comment
	for _, cm := range c.Comments {
		if cm.Path == path {
			out = append(out, cm)
		}
	}
	slices.SortFunc(out, byLine)
	return out
}

// AllComments returns every comment on a change, ordered by path and then by
// line, which is the order someone would work through them.
func (s *Store) AllComments(changeID string) []Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.changes[changeID]
	if c == nil {
		return nil
	}
	out := append([]Comment(nil), c.Comments...)
	slices.SortFunc(out, func(a, b Comment) int {
		if a.Path != b.Path {
			return cmp.Compare(a.Path, b.Path)
		}
		return byLine(a, b)
	})
	return out
}

// byLine orders notes the way someone would work through a file: down the
// lines, and within a line by when they were written.
func byLine(a, b Comment) int {
	if a.Line != b.Line {
		return cmp.Compare(a.Line, b.Line)
	}
	return a.CreatedAt.Compare(b.CreatedAt)
}

// OpenCount returns how many comments on a change are unresolved. The status
// bar asks every frame, so it counts in place rather than building a list.
func (s *Store) OpenCount(changeID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.changes[changeID]
	if c == nil {
		return 0
	}
	n := 0
	for _, cm := range c.Comments {
		if !cm.Resolved {
			n++
		}
	}
	return n
}

// CommentCount returns the number of comments on a path, and how many of those
// are unresolved.
func (s *Store) CommentCount(changeID, path string) (total, open int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.changes[changeID]
	if c == nil {
		return 0, 0
	}
	for _, cm := range c.Comments {
		if cm.Path == path {
			total++
			if !cm.Resolved {
				open++
			}
		}
	}
	return total, open
}

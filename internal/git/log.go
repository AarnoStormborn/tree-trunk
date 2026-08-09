package git

import (
	"bytes"
	"context"
	"time"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// logFormat is the NUL-separated field format for one commit record:
// short-hash, author, strict-ISO date, subject (03-git-layer.md §4.2).
// The trailing %x00 is intentionally ABSENT: `-z` already terminates each
// record with NUL (review m2 — a format-level %x00 would double it).
const logFormat = "--format=%h%x00%an%x00%aI%x00%s"

// LogOptions configures a paged `git log`.
type LogOptions struct {
	Skip  int    // records to skip (paging)
	Limit int    // max records (default 200)
	Path  string // restrict to a path ("" = all)
}

// Log runs `git log` in dir and parses up to Limit commits starting at Skip.
func Log(ctx context.Context, r Runner, dir string, opts LogOptions) ([]model.Commit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 200
	}
	args := []string{"log", logFormat, "-z", "-n", itoa(opts.Limit)}
	if opts.Skip > 0 {
		args = append(args, "--skip", itoa(opts.Skip))
	}
	if opts.Path != "" {
		args = append(args, "--", opts.Path)
	}
	out, err := r.RunIn(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return ParseLog(out)
}

// ParseLog parses the NUL-separated stream into commits. Fields are grouped
// in records of 4 (hash, author, date, subject); empty subjects are legal.
func ParseLog(out []byte) ([]model.Commit, error) {
	fields := bytes.Split(out, []byte{0})
	commits := make([]model.Commit, 0, len(fields)/4)
	for i := 0; i+3 < len(fields); i += 4 {
		hash := string(fields[i])
		if hash == "" {
			continue // stray separator (e.g. trailing NUL)
		}
		c := model.Commit{Hash: hash, Author: string(fields[i+1]), Subject: string(fields[i+3])}
		if t, err := time.Parse(time.RFC3339, string(fields[i+2])); err == nil {
			c.AuthorDate = t
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// itoa formats a non-negative int (no deps on strconv for hot paths).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

package strength

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The wire shapes below mirror 99-Strength's DTOs (A2S.Application/DTOs/
// HabitDtos.cs and BodyWeightDtos.cs). testdata/ holds one sample of each.

// WhoAmI is GET /users/me/api-tokens/whoami.
type WhoAmI struct {
	UserID    string `json:"userId"`
	Email     string `json:"email"`
	TokenName string `json:"tokenName"`
}

// RemoteHabit is HabitTodayDto. Type is "Counter", "Numeric" or "Checkbox".
type RemoteHabit struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Target     float64 `json:"target"`
	Minimum    float64 `json:"minimum"`
	Unit       *string `json:"unit"`
	Value      float64 `json:"value"`
	MetMinimum bool    `json:"metMinimum"`
	MetTarget  bool    `json:"metTarget"`
	Order      int     `json:"order"`
	Icon       *string `json:"icon"`
}

// RemoteStreak is HabitStreakDto.
type RemoteStreak struct {
	Current        int     `json:"current"`
	Best           int     `json:"best"`
	LastCountedDay *string `json:"lastCountedDay"`
	TodayCounted   bool    `json:"todayCounted"`
	GraceUsed      bool    `json:"graceUsed"`
}

// RemoteToday is HabitsTodayDto, the body of GET /habits/today.
type RemoteToday struct {
	Date   string        `json:"date"`
	Habits []RemoteHabit `json:"habits"`
	Streak RemoteStreak  `json:"streak"`
}

// RemoteLog is HabitLogDto, returned by both habit writes.
type RemoteLog struct {
	HabitID      string    `json:"habitId"`
	Date         string    `json:"date"`
	Value        float64   `json:"value"`
	UpdatedAtUTC time.Time `json:"updatedAtUtc"`
}

// RemoteWeight is BodyWeightEntryDto, returned by PUT /body-weight/{date}.
type RemoteWeight struct {
	Date          string    `json:"date"`
	Kg            float64   `json:"kg"`
	RecordedAtUTC time.Time `json:"recordedAtUtc"`
}

// HTTPError is an answer from 99-Strength that was not a 2xx.
type HTTPError struct {
	Status int
	Detail string // ProblemDetails "detail", when the body had one
}

func (e *HTTPError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("99-Strength answered %d: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("99-Strength answered %d %s", e.Status, http.StatusText(e.Status))
}

func statusOf(err error) int {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status
	}
	return 0
}

// unauthorized: the token is unknown or revoked.
func unauthorized(err error) bool { return statusOf(err) == http.StatusUnauthorized }

// retryable: no answer at all, or one that may be different next time. The
// same split the 99-Strength phone app makes (offline/errors.ts).
func retryable(err error) bool {
	st := statusOf(err)
	return st == 0 || st >= 500 || st == http.StatusRequestTimeout || st == http.StatusTooManyRequests
}

// remote calls the 99-Strength API with a personal API token.
type remote struct {
	base  string // e.g. https://api-99strength.enjoeneer.dev/api/v1
	token string
	http  *http.Client
}

func (r *remote) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var problem struct {
			Detail string `json:"detail"`
			Title  string `json:"title"`
		}
		json.Unmarshal(raw, &problem)
		if problem.Detail == "" {
			problem.Detail = problem.Title
		}
		return &HTTPError{Status: resp.StatusCode, Detail: problem.Detail}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("99-Strength sent something that is not the expected JSON: %w", err)
	}
	return nil
}

func (r *remote) whoAmI(ctx context.Context) (WhoAmI, error) {
	var w WhoAmI
	return w, r.do(ctx, "GET", "/users/me/api-tokens/whoami", nil, &w)
}

// today reads the habit screen for a London date (yyyy-MM-dd).
func (r *remote) today(ctx context.Context, date string) (RemoteToday, error) {
	var t RemoteToday
	return t, r.do(ctx, "GET", "/habits/today?date="+url.QueryEscape(date), nil, &t)
}

// stamp formats a client timestamp the way the phone app does (ISO, UTC, ms).
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// increment is the +1 / -1 tap. It is not idempotent: never send it twice.
func (r *remote) increment(ctx context.Context, habitID, date string, delta int, at time.Time) (RemoteLog, error) {
	var l RemoteLog
	body := map[string]any{"delta": delta, "clientTimestamp": stamp(at)}
	return l, r.do(ctx, "POST", "/habits/"+url.PathEscape(habitID)+"/logs/"+url.PathEscape(date)+"/increment", body, &l)
}

// setLog sets a day's value. Last write wins on clientTimestamp, so sending
// it again changes nothing.
func (r *remote) setLog(ctx context.Context, habitID, date string, value float64, at time.Time) (RemoteLog, error) {
	var l RemoteLog
	body := map[string]any{"value": value, "clientTimestamp": stamp(at)}
	return l, r.do(ctx, "PUT", "/habits/"+url.PathEscape(habitID)+"/logs/"+url.PathEscape(date), body, &l)
}

// recordWeight records or corrects the weigh-in for a date.
func (r *remote) recordWeight(ctx context.Context, date string, kg float64) (RemoteWeight, error) {
	var w RemoteWeight
	return w, r.do(ctx, "PUT", "/body-weight/"+url.PathEscape(date), map[string]any{"kg": kg}, &w)
}

// redact removes the token from text that is about to be stored or logged.
// Go's HTTP errors carry the URL, never headers, so this is a second lock.
func redact(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[token]")
}

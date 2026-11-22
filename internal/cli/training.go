package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"terrancc/internal/client"
	"terrancc/internal/strength"
)

// Training from 99-Strength: the "🏋" block in `tcc status` and `tcc workout`.

const futureNote = "Weights can change if you finish sessions before then."

func dayName(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("Mon 2 Jan")
}

func addDay(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// kg renders 75 as "75 kg" and 52.5 as "52.5 kg".
func kg(v float64) string { return num(v) + " kg" }

// weightText is how an exercise's set weight reads. 99-Strength sends 0
// both for bodyweight work and for a weight not set yet, so 0 reads
// "bodyweight" only when the name says so, "—" (set weight) for a main or
// auxiliary lift, which is always loaded, and nothing (reps only) otherwise.
func weightText(e strength.TrainingExercise, v float64) string {
	switch {
	case v > 0:
		return kg(v)
	case strings.Contains(strings.ToLower(e.Name), "bodyweight"):
		return "bodyweight"
	case needsWeight(e):
		return "—"
	}
	return ""
}

// needsWeight reports a loaded lift whose weight is not set yet.
func needsWeight(e strength.TrainingExercise) bool {
	if e.Category != "MainLift" && e.Category != "Auxiliary" {
		return false
	}
	for _, s := range e.Sets {
		if s.WeightKg > 0 {
			return false
		}
	}
	return len(e.Sets) > 0
}

// setText renders "75 kg × 5", or "75 kg × 10+ (AMRAP)" for an AMRAP set,
// whose reps are the rep-out target; "× 10" with no weight to show.
func setText(e strength.TrainingExercise, s strength.TrainingSet) string {
	w := weightText(e, s.WeightKg)
	if w != "" {
		w += " "
	}
	if s.AMRAP {
		return fmt.Sprintf("%s× %d+ (AMRAP)", w, s.Reps)
	}
	return fmt.Sprintf("%s× %d", w, s.Reps)
}

var equipment = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// shortExercise drops the equipment: "Bench Press (Barbell)" is "Bench Press".
func shortExercise(name string) string {
	if s := equipment.ReplaceAllString(name, ""); s != "" {
		return s
	}
	return name
}

// liftSummary is the headline of a lift: its first main lift (or first
// exercise) as "Bench Press 5×5+ @ 75 kg" ("+" when the last set is AMRAP).
func liftSummary(l *strength.TrainingLift) string {
	if l == nil || len(l.Exercises) == 0 {
		return ""
	}
	e := l.Exercises[0]
	for _, x := range l.Exercises {
		if x.Category == "MainLift" {
			e = x
			break
		}
	}
	if len(e.Sets) == 0 {
		return shortExercise(e.Name)
	}
	first, last := e.Sets[0], e.Sets[len(e.Sets)-1]
	plus := ""
	if last.AMRAP {
		plus = "+"
	}
	top := 0.0
	for _, s := range e.Sets {
		top = max(top, s.WeightKg)
	}
	head := fmt.Sprintf("%s %d×%d%s", shortExercise(e.Name), len(e.Sets), first.Reps, plus)
	if w := weightText(e, top); w != "" {
		head += " @ " + w
	}
	return head
}

// finisherShort is "Stair 10 min" from "Stair finisher" and "10 min · steady, level 8".
func finisherShort(f *strength.TrainingFinisher) string {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(f.Name), "finisher"))
	if name == "" {
		name = f.Name
	}
	dose, _, _ := strings.Cut(f.Prescription, "·")
	return strings.TrimSpace(name + " " + strings.TrimSpace(dose))
}

// statusMark is how a session's status reads after its label.
func statusMark(status string) string {
	switch status {
	case "done":
		return " ✓"
	case "planned", "":
		return " (planned)"
	case "rest":
		return ""
	default:
		return " (" + status + ")"
	}
}

func kindIcon(kind string) string {
	switch kind {
	case "lift":
		return "🏋"
	case "cardio":
		return "🏃"
	case "mobility":
		return "🧘"
	default:
		return "💤"
	}
}

// openWork reports whether a day still has a session to do.
func openWork(d strength.TrainingDay) bool {
	for _, s := range d.Sessions {
		if s.Kind != "rest" && s.Status == "planned" {
			return true
		}
	}
	return false
}

// nextPlanned is the first day after today with a session to do.
func nextPlanned(t strength.Training) *strength.TrainingDay {
	for i := range t.Days {
		if d := &t.Days[i]; d.Date > t.Today && openWork(*d) {
			return d
		}
	}
	return nil
}

func findDay(t strength.Training, date string) *strength.TrainingDay {
	for i := range t.Days {
		if t.Days[i].Date == date {
			return &t.Days[i]
		}
	}
	return nil
}

// dayHeadline is "Upper A (planned) · finisher: Stair 10 min" or "Rest".
func dayHeadline(d strength.TrainingDay) string {
	if d.Rest {
		return "Rest"
	}
	var parts []string
	for _, s := range d.Sessions {
		if s.Kind == "rest" {
			continue
		}
		p := s.Label + statusMark(s.Status)
		if s.Lift != nil && s.Lift.Finisher != nil {
			p += " · finisher: " + finisherShort(s.Lift.Finisher)
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " · ")
}

// upNextText is "Tue 24 Nov Upper B — Bench Press 5×5+ @ 75 kg".
func upNextText(d strength.TrainingDay) string {
	var parts []string
	for _, s := range d.Sessions {
		if s.Kind == "rest" || s.Status != "planned" {
			continue
		}
		p := s.Label
		if sum := liftSummary(s.Lift); sum != "" {
			p += " — " + sum
		}
		parts = append(parts, p)
	}
	return dayName(d.Date) + " " + strings.Join(parts, " · ")
}

// trainingNote is the warning under the training line, or "".
func trainingNote(sy *strength.Sync) string {
	switch {
	case sy == nil || !sy.Stale:
		return ""
	case sy.NeedsToken:
		return "⚠ 99-Strength token needs renewing: run `tcc strength connect`. Showing the plan from " + syncTime(sy.LastSync)
	default:
		return "⚠ 99-Strength is offline. Showing the plan from " + syncTime(sy.LastSync)
	}
}

// trainingLines is the status block: today, and the next planned day when
// nothing is left to do today.
func trainingLines(t strength.Training) []string {
	if t.NoPlan {
		return []string{"🏋 Training: no active plan in 99-Strength"}
	}
	line := "🏋 Today: nothing planned"
	today := findDay(t, t.Today)
	if today != nil {
		line = "🏋 Today: " + dayHeadline(*today)
	}
	if today == nil || !openWork(*today) {
		if next := nextPlanned(t); next != nil {
			line += " · Up next: " + upNextText(*next)
		}
	}
	out := []string{line}
	if note := trainingNote(t.Sync); note != "" {
		out = append(out, "  "+note)
	}
	return out
}

// statusTraining reads training for `tcc status`. Not connected is no
// block at all; any other failure is one line rather than a failed status.
func statusTraining(c *client.Client) (*strength.Training, []string) {
	t, err := c.Training("", 0)
	if err != nil {
		var ae *client.APIError
		if errors.As(err, &ae) && ae.Status == 409 {
			return nil, nil
		}
		return nil, []string{"🏋 Training: " + err.Error()}
	}
	return &t, trainingLines(t)
}

// printDay prints every session of a day, a lift with all its sets.
func (a *app) printDay(d strength.TrainingDay) {
	head := dayName(d.Date)
	if d.Phase != "" {
		head += " · " + strings.ToUpper(d.Phase[:1]) + d.Phase[1:] + " phase"
	}
	a.printf("%s\n", head)
	if d.Rest || len(d.Sessions) == 0 {
		a.printf("  💤 Rest day\n")
		return
	}
	for _, s := range d.Sessions {
		if s.Kind == "rest" {
			continue
		}
		line := fmt.Sprintf("  %s %s%s", kindIcon(s.Kind), s.Label, statusMark(s.Status))
		if s.Lift != nil {
			line += fmt.Sprintf(" · week %d", s.Lift.Week)
		}
		a.printf("%s\n", line)
		if s.Lift == nil {
			continue
		}
		if s.Lift.Deload {
			a.printf("    Deload week: lighter weights and fewer sets, no AMRAP\n")
		}
		for _, e := range s.Lift.Exercises {
			sets := make([]string, len(e.Sets))
			for i, x := range e.Sets {
				sets[i] = setText(e, x)
			}
			name := e.Name
			if needsWeight(e) {
				name += " (set weight in 99-Strength)"
			}
			a.printf("    %s\n      %s\n", name, strings.Join(sets, " · "))
		}
		if f := s.Lift.Finisher; f != nil {
			a.printf("    Finisher: %s — %s\n", f.Name, f.Prescription)
		}
	}
}

// workoutOut is what `tcc workout --json` prints.
type workoutOut struct {
	Asked  string                `json:"asked"`
	Today  string                `json:"today"`
	Day    *strength.TrainingDay `json:"day"`
	NoPlan bool                  `json:"no_plan"`
	Note   string                `json:"note,omitempty"`
	Future bool                  `json:"future"` // weights may still change
	Sync   *strength.Sync        `json:"sync,omitempty"`
}

func (a *app) workoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "workout [today|tomorrow|next|YYYY-MM-DD]",
		Short: "What's my workout? Sessions and sets from 99-Strength",
		Long: "Show a day's training from 99-Strength: each session, and for a lift every\n" +
			"exercise with its sets (\"75 kg × 5\"; an AMRAP set shows its rep-out target as\n" +
			"\"75 kg × 10+ (AMRAP)\"). With no argument it is today, or the next planned day\n" +
			"when today is a rest day. `next` is the next day with something planned.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			asked := "today"
			if len(args) == 1 {
				asked = strings.ToLower(args[0])
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			t, err := c.Training("", 0)
			if err != nil {
				return err
			}
			out := workoutOut{Asked: asked, Today: t.Today, NoPlan: t.NoPlan, Sync: t.Sync}
			var date string
			switch asked {
			case "today":
				date = t.Today
				if d := findDay(t, date); d != nil && d.Rest {
					if next := nextPlanned(t); next != nil {
						out.Note = "Today is a rest day. Next planned day:"
						out.Day = next
					}
				}
			case "tomorrow":
				date = addDay(t.Today, 1)
			case "next":
				out.Day = nextPlanned(t)
				if out.Day == nil && !t.NoPlan {
					out.Note = "Nothing planned in the next two weeks."
				}
			default:
				if _, err := time.Parse("2006-01-02", asked); err != nil {
					return fmt.Errorf("%q is not a day: use today, tomorrow, next or YYYY-MM-DD", args[0])
				}
				date = asked
			}
			if out.Day == nil && date != "" && !t.NoPlan {
				out.Day = findDay(t, date)
				if out.Day == nil { // outside the two weeks tccd keeps
					one, err := c.Training(date, 1)
					if err != nil {
						return err
					}
					out.Sync = one.Sync
					if len(one.Days) > 0 {
						out.Day = &one.Days[0]
					}
				}
			}
			out.Future = out.Day != nil && out.Day.Date > t.Today
			return a.emit(out, func() {
				switch {
				case t.NoPlan:
					a.printf("No active training plan in 99-Strength.\n")
					return
				case out.Day == nil && out.Note != "":
					a.printf("%s\n", out.Note)
					return
				case out.Day == nil:
					a.printf("%s: nothing in the plan.\n", dayName(date))
					return
				}
				if out.Note != "" {
					a.printf("%s\n", out.Note)
				}
				a.printDay(*out.Day)
				if out.Future && openWork(*out.Day) {
					a.printf("%s\n", futureNote)
				}
				if note := trainingNote(out.Sync); note != "" {
					a.printf("%s\n", note)
				}
			})
		},
	}
}

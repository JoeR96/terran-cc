package model

import "testing"

func TestEnumsValid(t *testing.T) {
	for _, k := range []Kind{KindTodo, KindBug, KindTask, KindFeature, KindIdea} {
		if !k.Valid() {
			t.Errorf("kind %q should be valid", k)
		}
	}
	for _, s := range []Status{StatusInbox, StatusBacklog, StatusUpNext, StatusInProgress, StatusReview, StatusDone, StatusSomeday} {
		if !s.Valid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	for _, w := range []When{WhenToday, WhenWeek, WhenMonth, WhenAnytime} {
		if !w.Valid() {
			t.Errorf("when %q should be valid", w)
		}
	}
	for _, c := range []Context{ContextWork, ContextIRL} {
		if !c.Valid() {
			t.Errorf("context %q should be valid", c)
		}
	}
	for _, h := range []HabitType{HabitCount, HabitAmount, HabitCheck} {
		if !h.Valid() {
			t.Errorf("habit type %q should be valid", h)
		}
	}
	if Kind("nope").Valid() || Status("nope").Valid() || When("nope").Valid() || Context("nope").Valid() || HabitType("nope").Valid() {
		t.Error("nope should be invalid")
	}
	if !(Evidence{Type: "commit"}).Valid() {
		t.Error("commit evidence should be valid")
	}
	if (Evidence{Type: "x"}).Valid() {
		t.Error("x evidence should be invalid")
	}
}

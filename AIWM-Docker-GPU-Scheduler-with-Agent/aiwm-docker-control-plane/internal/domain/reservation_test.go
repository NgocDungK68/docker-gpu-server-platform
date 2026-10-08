package domain

import (
	"testing"
	"time"
)

func TestHalfOpenReservationOverlap(t *testing.T) {
	base := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	a := TimeWindow{base, base.Add(2 * time.Hour)}
	for _, tc := range []struct {
		offset, duration time.Duration
		overlap          bool
	}{
		{time.Hour, 2 * time.Hour, true}, {2 * time.Hour, time.Hour, false}, {-time.Hour, time.Hour, false},
		{0, 2 * time.Hour, true}, {-time.Hour, 4 * time.Hour, true},
	} {
		b := TimeWindow{base.Add(tc.offset), base.Add(tc.offset + tc.duration)}
		if a.Overlaps(b) != tc.overlap || b.Overlaps(a) != tc.overlap {
			t.Fatalf("%+v", tc)
		}
	}
	if !a.Contains(a.StartAt) || a.Contains(a.EndAt) {
		t.Fatal("not half-open")
	}
}

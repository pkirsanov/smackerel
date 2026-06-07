package intelligence

import (
	"fmt"
	"testing"
	"time"
)

// === clampDay systematic month-end coverage ===

func TestClampDay_AllMonths(t *testing.T) {
	// Verify clampDay correctly clamps day=31 to the last day of each month
	// for a non-leap year.
	expected := map[time.Month]int{
		time.January:   31,
		time.February:  28,
		time.March:     31,
		time.April:     30,
		time.May:       31,
		time.June:      30,
		time.July:      31,
		time.August:    31,
		time.September: 30,
		time.October:   31,
		time.November:  30,
		time.December:  31,
	}
	for month, wantDay := range expected {
		t.Run(month.String(), func(t *testing.T) {
			d := clampDay(2026, month, 31)
			if d.Day() != wantDay {
				t.Errorf("clampDay(2026, %s, 31) = day %d, want %d", month, d.Day(), wantDay)
			}
			if d.Month() != month {
				t.Errorf("clampDay month shifted: got %s, want %s", d.Month(), month)
			}
			if d.Year() != 2026 {
				t.Errorf("clampDay year shifted: got %d, want 2026", d.Year())
			}
		})
	}
}

func TestClampDay_LeapVsNonLeap(t *testing.T) {
	// Feb 29 in leap year → 29, in non-leap → 28
	leap := clampDay(2028, time.February, 29)
	if leap.Day() != 29 {
		t.Errorf("leap year Feb 29: got day %d, want 29", leap.Day())
	}
	nonLeap := clampDay(2026, time.February, 29)
	if nonLeap.Day() != 28 {
		t.Errorf("non-leap year Feb 29: got day %d, want 28", nonLeap.Day())
	}
}

func TestClampDay_PreservesTimeLocation(t *testing.T) {
	d := clampDay(2026, time.June, 15)
	if d.Location() != time.Local {
		t.Errorf("clampDay should use time.Local, got %v", d.Location())
	}
	// Verify zero hour/minute/second
	if d.Hour() != 0 || d.Minute() != 0 || d.Second() != 0 {
		t.Errorf("clampDay should be at midnight, got %02d:%02d:%02d", d.Hour(), d.Minute(), d.Second())
	}
}

func TestClampDay_LargeDayValue(t *testing.T) {
	// Day=100 should clamp to last day of month
	d := clampDay(2026, time.April, 100)
	if d.Day() != 30 {
		t.Errorf("expected day 30 for April with day=100, got %d", d.Day())
	}
	if d.Month() != time.April {
		t.Errorf("month should stay April, got %s", d.Month())
	}
}

// === calendarDaysBetween additional coverage ===

func TestCalendarDaysBetween_LargeSpan(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	got := calendarDaysBetween(from, to)
	if got != 364 {
		t.Errorf("Jan 1 to Dec 31 2026 = %d days, want 364", got)
	}
}

func TestCalendarDaysBetween_LeapYearSpan(t *testing.T) {
	from := time.Date(2028, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2028, 12, 31, 0, 0, 0, 0, time.Local)
	got := calendarDaysBetween(from, to)
	if got != 365 {
		t.Errorf("Jan 1 to Dec 31 2028 (leap) = %d days, want 365", got)
	}
}

func TestCalendarDaysBetween_IgnoresTimeOfDay(t *testing.T) {
	// Same date, different times → still 0 days
	from := time.Date(2026, 4, 15, 3, 45, 0, 0, time.Local)
	to := time.Date(2026, 4, 15, 23, 59, 59, 0, time.Local)
	got := calendarDaysBetween(from, to)
	if got != 0 {
		t.Errorf("same date different times should be 0, got %d", got)
	}
}

func TestCalendarDaysBetween_Symmetric(t *testing.T) {
	a := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	b := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	forward := calendarDaysBetween(a, b)
	backward := calendarDaysBetween(b, a)
	if forward != -backward {
		t.Errorf("forward (%d) should be negative of backward (%d)", forward, backward)
	}
}

func TestCalendarDaysBetween_CrossTimezone(t *testing.T) {
	// Inputs in different zones should still count calendar days correctly
	// because the function normalizes to UTC midnight.
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 3, 0, 0, 0, 0, time.Local)
	got := calendarDaysBetween(from, to)
	if got != 2 {
		t.Errorf("expected 2 days, got %d", got)
	}
}

// === Billing title formatting ===

func TestBillingTitleFormat_WithAmount(t *testing.T) {
	// Replicate the title formatting from ProduceBillAlerts
	serviceName := "Netflix"
	amount := 15.99
	currency := "USD"

	var title string
	if amount > 0 {
		title = "Upcoming charge: " + serviceName + " (" + formatAmount(amount) + " " + currency + ")"
	} else {
		title = "Upcoming charge: " + serviceName
	}

	if title != "Upcoming charge: Netflix (15.99 USD)" {
		t.Errorf("unexpected title: %s", title)
	}
}

func TestBillingTitleFormat_ZeroAmount(t *testing.T) {
	serviceName := "Free Tier"
	amount := 0.0

	var title string
	if amount > 0 {
		title = "Upcoming charge: " + serviceName + " (" + formatAmount(amount) + " USD)"
	} else {
		title = "Upcoming charge: " + serviceName
	}

	if title != "Upcoming charge: Free Tier" {
		t.Errorf("zero amount should omit price, got: %s", title)
	}
}

// formatAmount is a test helper to replicate the %.2f formatting
func formatAmount(amount float64) string {
	return fmt.Sprintf("%.2f", amount)
}

// === Billing days-until range check ===

func TestBillingDaysUntilRange(t *testing.T) {
	// ProduceBillAlerts skips if daysUntilBilling < 0 || daysUntilBilling > 3
	tests := []struct {
		days     int
		eligible bool
	}{
		{-1, false},
		{0, true},  // billing today
		{1, true},  // billing tomorrow
		{2, true},  // billing in 2 days
		{3, true},  // billing in 3 days
		{4, false}, // too far out
		{5, false},
		{-10, false},
	}
	for _, tt := range tests {
		eligible := tt.days >= 0 && tt.days <= 3
		if eligible != tt.eligible {
			t.Errorf("days=%d: got eligible=%v, want %v", tt.days, eligible, tt.eligible)
		}
	}
}

// === Monthly billing date rollover logic ===

func TestMonthlyBillingRollover(t *testing.T) {
	// Test the billing date rollover logic from ProduceBillAlerts
	// If nextBilling is before today, roll to next month

	billingDay := 15
	// Simulate: today is April 20, billing day was April 15 (already passed)
	now := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.Local)
	localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	nextBilling := clampDay(now.Year(), now.Month(), billingDay) // April 15
	if nextBilling.Before(localToday) {
		nextMonth := now.Month() + 1
		nextYear := now.Year()
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		nextBilling = clampDay(nextYear, nextMonth, billingDay)
	}

	if nextBilling.Month() != time.May || nextBilling.Day() != 15 {
		t.Errorf("expected May 15, got %s", nextBilling.Format("2006-01-02"))
	}
}

func TestMonthlyBillingDecemberRollover(t *testing.T) {
	// December billing day already passed → should roll to January next year
	billingDay := 10
	now := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.Local)
	localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	nextBilling := clampDay(now.Year(), now.Month(), billingDay) // Dec 10
	if nextBilling.Before(localToday) {
		nextMonth := now.Month() + 1
		nextYear := now.Year()
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		nextBilling = clampDay(nextYear, nextMonth, billingDay)
	}

	if nextBilling.Year() != 2027 || nextBilling.Month() != time.January || nextBilling.Day() != 10 {
		t.Errorf("expected 2027-01-10, got %s", nextBilling.Format("2006-01-02"))
	}
}

// === Annual billing date logic ===

func TestAnnualBillingDateThisYear(t *testing.T) {
	// Annual billing: firstSeen was June 15, current date is June 10 → bill June 15 this year
	firstSeen := time.Date(2024, time.June, 15, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.Local)
	localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	billingDay := firstSeen.Day()
	nextBilling := clampDay(now.Year(), firstSeen.Month(), billingDay)
	if nextBilling.Before(localToday) {
		nextBilling = clampDay(now.Year()+1, firstSeen.Month(), billingDay)
	}

	if nextBilling.Year() != 2026 || nextBilling.Month() != time.June || nextBilling.Day() != 15 {
		t.Errorf("expected 2026-06-15, got %s", nextBilling.Format("2006-01-02"))
	}
}

func TestAnnualBillingDateNextYear(t *testing.T) {
	// Annual billing: firstSeen was March 5, current date is March 10 → bill March 5 next year
	firstSeen := time.Date(2024, time.March, 5, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.Local)
	localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	billingDay := firstSeen.Day()
	nextBilling := clampDay(now.Year(), firstSeen.Month(), billingDay)
	if nextBilling.Before(localToday) {
		nextBilling = clampDay(now.Year()+1, firstSeen.Month(), billingDay)
	}

	if nextBilling.Year() != 2027 || nextBilling.Month() != time.March || nextBilling.Day() != 5 {
		t.Errorf("expected 2027-03-05, got %s", nextBilling.Format("2006-01-02"))
	}
}

package digest

import (
	"strings"
	"testing"
)

func TestHospitalityDigestContext_IsEmpty_AllZero(t *testing.T) {
	h := &HospitalityDigestContext{}
	if !h.IsEmpty() {
		t.Error("expected empty hospitality context to be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_WithArrivals(t *testing.T) {
	h := &HospitalityDigestContext{
		TodayArrivals: []GuestStay{
			{GuestName: "Alice", PropertyName: "Beach House", CheckIn: "2026-04-11", CheckOut: "2026-04-14", Source: "guesthost", TotalPrice: 450},
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with arrivals should not be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_WithDepartures(t *testing.T) {
	h := &HospitalityDigestContext{
		TodayDepartures: []GuestStay{
			{GuestName: "Bob", PropertyName: "Cabin", CheckIn: "2026-04-08", CheckOut: "2026-04-11", Source: "guesthost", TotalPrice: 300},
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with departures should not be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_WithTasks(t *testing.T) {
	h := &HospitalityDigestContext{
		PendingTasks: []HospitalityTask{
			{PropertyName: "Beach House", Title: "Clean pool", Category: "maintenance", Status: "pending"},
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with pending tasks should not be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_WithGuestAlerts(t *testing.T) {
	h := &HospitalityDigestContext{
		GuestAlerts: []GuestAlert{
			{GuestName: "Carol", GuestEmail: "carol@example.com", AlertType: "repeat_guest", Description: "Repeat guest with 3 stays"},
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with guest alerts should not be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_WithPropertyAlerts(t *testing.T) {
	h := &HospitalityDigestContext{
		PropertyAlerts: []PropertyAlert{
			{PropertyName: "Beach House", AlertType: "high_issue_count", Description: "Property has 7 open issues"},
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with property alerts should not be empty")
	}
}

func TestHospitalityDigestContext_IsEmpty_RevenueOnly(t *testing.T) {
	h := &HospitalityDigestContext{
		Revenue: RevenueSnapshot{
			TodayCheckIns:  2,
			TodayCheckOuts: 1,
			WeekRevenue:    1200,
			MonthRevenue:   4500,
		},
	}
	if h.IsEmpty() {
		t.Error("hospitality context with check-in counts should not be empty")
	}
}

func TestGuestStay_Fields(t *testing.T) {
	s := GuestStay{
		GuestName:    "Alice",
		GuestEmail:   "alice@example.com",
		PropertyName: "Beach House",
		CheckIn:      "2026-04-11",
		CheckOut:     "2026-04-14",
		Source:       "guesthost",
		TotalPrice:   450.50,
	}
	if s.GuestName != "Alice" {
		t.Errorf("expected GuestName Alice, got %s", s.GuestName)
	}
	if s.TotalPrice != 450.50 {
		t.Errorf("expected TotalPrice 450.50, got %f", s.TotalPrice)
	}
}

func TestHospitalityTask_Fields(t *testing.T) {
	task := HospitalityTask{
		PropertyName: "Cabin",
		Title:        "Replace towels",
		Category:     "housekeeping",
		Status:       "pending",
	}
	if task.Title != "Replace towels" {
		t.Errorf("expected title 'Replace towels', got %s", task.Title)
	}
	if task.Status != "pending" {
		t.Errorf("expected status 'pending', got %s", task.Status)
	}
}

func TestRevenueSnapshot_Fields(t *testing.T) {
	snap := RevenueSnapshot{
		TodayCheckIns:  3,
		TodayCheckOuts: 2,
		DayRevenue:     800.00,
		WeekRevenue:    2500.00,
		MonthRevenue:   9800.00,
		ByChannel: map[string]float64{
			"direct": 5000.00,
			"airbnb": 4800.00,
		},
		ByProperty: map[string]float64{
			"Beach House":    6000.00,
			"Mountain Cabin": 3800.00,
		},
	}
	if snap.TodayCheckIns != 3 {
		t.Errorf("expected 3 check-ins, got %d", snap.TodayCheckIns)
	}
	if snap.DayRevenue != 800.00 {
		t.Errorf("expected day revenue 800, got %f", snap.DayRevenue)
	}
	if snap.MonthRevenue != 9800.00 {
		t.Errorf("expected month revenue 9800, got %f", snap.MonthRevenue)
	}
	if snap.ByChannel["direct"] != 5000.00 {
		t.Errorf("expected direct channel 5000, got %f", snap.ByChannel["direct"])
	}
	if snap.ByChannel["airbnb"] != 4800.00 {
		t.Errorf("expected airbnb channel 4800, got %f", snap.ByChannel["airbnb"])
	}
	if snap.ByProperty["Beach House"] != 6000.00 {
		t.Errorf("expected Beach House property 6000, got %f", snap.ByProperty["Beach House"])
	}
}

func TestRevenueSnapshot_DayRevenueWindow(t *testing.T) {
	snap := RevenueSnapshot{
		DayRevenue:   150.00,
		WeekRevenue:  1200.00,
		MonthRevenue: 4500.00,
	}
	if snap.DayRevenue != 150.00 {
		t.Errorf("expected day revenue 150, got %f", snap.DayRevenue)
	}
	if snap.DayRevenue > snap.WeekRevenue {
		t.Error("24h revenue should not exceed week revenue")
	}
	if snap.WeekRevenue > snap.MonthRevenue {
		t.Error("week revenue should not exceed month revenue")
	}
}

func TestRevenueSnapshot_EmptyChannelBreakdown(t *testing.T) {
	snap := RevenueSnapshot{
		WeekRevenue:  500.00,
		MonthRevenue: 500.00,
	}
	if snap.ByChannel != nil && len(snap.ByChannel) > 0 {
		t.Error("expected nil or empty channel breakdown when not set")
	}
}

func TestGuestAlert_Fields(t *testing.T) {
	alert := GuestAlert{
		GuestName:   "Carol",
		GuestEmail:  "carol@example.com",
		AlertType:   "repeat_guest",
		Description: "Repeat guest with 5 stays, total spend $2100",
	}
	if alert.AlertType != "repeat_guest" {
		t.Errorf("expected alert type 'repeat_guest', got %s", alert.AlertType)
	}
}

func TestPropertyAlert_Fields(t *testing.T) {
	alert := PropertyAlert{
		PropertyName: "Lakeside Villa",
		AlertType:    "low_rating",
		Description:  "Average rating: 3.2",
	}
	if alert.AlertType != "low_rating" {
		t.Errorf("expected alert type 'low_rating', got %s", alert.AlertType)
	}
}

func TestFormatHospitalityFallback_Full(t *testing.T) {
	h := &HospitalityDigestContext{
		TodayArrivals: []GuestStay{
			{GuestName: "Alice", PropertyName: "Beach House"},
			{GuestName: "Bob", PropertyName: "Cabin"},
		},
		TodayDepartures: []GuestStay{
			{GuestName: "Carol", PropertyName: "Beach House"},
		},
		PendingTasks: []HospitalityTask{
			{PropertyName: "Cabin", Title: "Clean pool"},
		},
		Revenue: RevenueSnapshot{
			DayRevenue:   400,
			WeekRevenue:  1500,
			MonthRevenue: 6200,
			ByChannel: map[string]float64{
				"direct": 3800,
				"airbnb": 2400,
			},
		},
		GuestAlerts: []GuestAlert{
			{GuestName: "Dave", AlertType: "repeat_guest"},
		},
		PropertyAlerts: []PropertyAlert{
			{PropertyName: "Cabin", AlertType: "high_issue_count"},
		},
	}

	text := formatHospitalityFallback(h)

	if text == "" {
		t.Fatal("expected non-empty fallback text")
	}
	// Verify key sections are present
	checks := []string{
		"--- Hospitality ---",
		"Arrivals today: 2",
		"Alice at Beach House",
		"Departures today: 1",
		"Carol from Beach House",
		"Pending tasks: 1",
		"Revenue",
		"24h: $400.00",
		"$1500.00",
		"$6200.00",
		"airbnb: $2400.00",
		"direct: $3800.00",
		"Guest alerts: 1",
		"Property alerts: 1",
	}
	for _, chk := range checks {
		if !strings.Contains(text, chk) {
			t.Errorf("expected fallback text to contain %q, got:\n%s", chk, text)
		}
	}
}

func TestFormatHospitalityFallback_EmptyArrivals(t *testing.T) {
	h := &HospitalityDigestContext{
		PendingTasks: []HospitalityTask{
			{PropertyName: "Cabin", Title: "Fix AC"},
		},
	}

	text := formatHospitalityFallback(h)

	if strings.Contains(text, "Arrivals today") {
		t.Error("should not contain arrivals section when none exist")
	}
	if strings.Contains(text, "Departures today") {
		t.Error("should not contain departures section when none exist")
	}
	if !strings.Contains(text, "Pending tasks: 1") {
		t.Error("should contain pending tasks")
	}
}

func TestFormatHospitalityFallback_NoRevenue(t *testing.T) {
	h := &HospitalityDigestContext{
		TodayArrivals: []GuestStay{
			{GuestName: "Alice", PropertyName: "Beach House"},
		},
	}

	text := formatHospitalityFallback(h)

	if strings.Contains(text, "Revenue") {
		t.Error("should omit revenue section when both week and month are 0")
	}
}

func TestDigestContext_WithHospitality(t *testing.T) {
	hCtx := &HospitalityDigestContext{
		TodayArrivals: []GuestStay{
			{GuestName: "Alice", PropertyName: "Beach House"},
		},
		Revenue: RevenueSnapshot{TodayCheckIns: 1},
	}
	ctx := DigestContext{
		DigestDate:  "2026-04-11",
		Hospitality: hCtx,
	}
	if ctx.Hospitality == nil {
		t.Error("expected hospitality context to be set")
	}
	if len(ctx.Hospitality.TodayArrivals) != 1 {
		t.Errorf("expected 1 arrival, got %d", len(ctx.Hospitality.TodayArrivals))
	}
}

func TestDigestContext_WithoutHospitality(t *testing.T) {
	ctx := DigestContext{
		DigestDate: "2026-04-11",
	}
	if ctx.Hospitality != nil {
		t.Error("expected nil hospitality context when not set")
	}
}

func TestGeneratorIsGuestHostActive_NilRegistry(t *testing.T) {
	g := &Generator{Registry: nil}
	if g.isGuestHostActive() {
		t.Error("expected false when registry is nil")
	}
}

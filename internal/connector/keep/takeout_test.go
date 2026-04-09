package keep

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestJSON(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test JSON %s: %v", path, err)
	}
	return path
}

func TestParseTextNote(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "text-note.json", `{
		"color": "DEFAULT",
		"isTrashed": false,
		"isPinned": false,
		"isArchived": false,
		"textContent": "Remember to buy milk",
		"title": "Grocery Reminder",
		"userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [{"name": "Shopping"}],
		"annotations": [],
		"attachments": [],
		"listContent": [],
		"sharees": []
	}`)

	parser := NewTakeoutParser()
	note, err := parser.ParseNoteFile(filepath.Join(dir, "text-note.json"))
	if err != nil {
		t.Fatalf("parse text note: %v", err)
	}

	if note.Title != "Grocery Reminder" {
		t.Errorf("title = %q, want %q", note.Title, "Grocery Reminder")
	}
	if note.TextContent != "Remember to buy milk" {
		t.Errorf("text content = %q, want %q", note.TextContent, "Remember to buy milk")
	}
	if len(note.Labels) != 1 || note.Labels[0].Name != "Shopping" {
		t.Errorf("labels = %v, want [{Shopping}]", note.Labels)
	}
}

func TestParseChecklistNote(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "checklist.json", `{
		"color": "BLUE",
		"isTrashed": false,
		"isPinned": false,
		"isArchived": false,
		"textContent": "",
		"title": "Packing List",
		"userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [],
		"annotations": [],
		"attachments": [],
		"listContent": [
			{"text": "Passport", "isChecked": true},
			{"text": "Charger", "isChecked": true},
			{"text": "Sunscreen", "isChecked": true},
			{"text": "Swimsuit", "isChecked": false},
			{"text": "Camera", "isChecked": false}
		],
		"sharees": []
	}`)

	parser := NewTakeoutParser()
	note, err := parser.ParseNoteFile(filepath.Join(dir, "checklist.json"))
	if err != nil {
		t.Fatalf("parse checklist: %v", err)
	}

	if len(note.ListContent) != 5 {
		t.Fatalf("list items = %d, want 5", len(note.ListContent))
	}
	checked := 0
	for _, item := range note.ListContent {
		if item.IsChecked {
			checked++
		}
	}
	if checked != 3 {
		t.Errorf("checked items = %d, want 3", checked)
	}
}

func TestParseImageNote(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "image-note.json", `{
		"color": "DEFAULT",
		"isTrashed": false,
		"isPinned": false,
		"isArchived": false,
		"textContent": "Whiteboard from standup",
		"title": "Meeting Notes",
		"userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [],
		"annotations": [],
		"attachments": [
			{"filePath": "photo1.jpg", "mimetype": "image/jpeg"},
			{"filePath": "photo2.png", "mimetype": "image/png"}
		],
		"listContent": [],
		"sharees": []
	}`)

	parser := NewTakeoutParser()
	note, err := parser.ParseNoteFile(filepath.Join(dir, "image-note.json"))
	if err != nil {
		t.Fatalf("parse image note: %v", err)
	}

	if len(note.Attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(note.Attachments))
	}
	if note.Attachments[0].MimeType != "image/jpeg" {
		t.Errorf("attachment[0] mimetype = %q, want image/jpeg", note.Attachments[0].MimeType)
	}
}

func TestParseAudioNote(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "audio-note.json", `{
		"color": "DEFAULT",
		"isTrashed": false,
		"isPinned": false,
		"isArchived": false,
		"textContent": "Transcribed voice memo text",
		"title": "Voice Memo",
		"userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [],
		"annotations": [],
		"attachments": [
			{"filePath": "recording.3gp", "mimetype": "audio/3gpp"}
		],
		"listContent": [],
		"sharees": []
	}`)

	parser := NewTakeoutParser()
	note, err := parser.ParseNoteFile(filepath.Join(dir, "audio-note.json"))
	if err != nil {
		t.Fatalf("parse audio note: %v", err)
	}

	if len(note.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(note.Attachments))
	}
	if note.Attachments[0].MimeType != "audio/3gpp" {
		t.Errorf("mimetype = %q, want audio/3gpp", note.Attachments[0].MimeType)
	}
}

func TestParseMixedNote(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "mixed-note.json", `{
		"color": "DEFAULT",
		"isTrashed": false,
		"isPinned": false,
		"isArchived": false,
		"textContent": "Team reorg ideas",
		"title": "Project Planning",
		"userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [],
		"annotations": [],
		"attachments": [
			{"filePath": "diagram.jpg", "mimetype": "image/jpeg"}
		],
		"listContent": [
			{"text": "Notify HR", "isChecked": false}
		],
		"sharees": []
	}`)

	parser := NewTakeoutParser()
	note, err := parser.ParseNoteFile(filepath.Join(dir, "mixed-note.json"))
	if err != nil {
		t.Fatalf("parse mixed note: %v", err)
	}

	if note.TextContent == "" {
		t.Error("expected text content to be non-empty")
	}
	if len(note.ListContent) != 1 {
		t.Errorf("list items = %d, want 1", len(note.ListContent))
	}
	if len(note.Attachments) != 1 {
		t.Errorf("attachments = %d, want 1", len(note.Attachments))
	}
}

func TestParseExportDirectory(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTestJSON(t, dir, filepath.Base(t.Name())+"-"+string(rune('a'+i))+".json", `{
			"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
			"textContent": "Content", "title": "Note",
			"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
			"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
		}`)
	}

	parser := NewTakeoutParser()
	notes, errors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if len(notes) != 5 {
		t.Errorf("notes = %d, want 5", len(notes))
	}
	if len(errors) != 0 {
		t.Errorf("errors = %d, want 0", len(errors))
	}
}

func TestParseExportWithCorrupted(t *testing.T) {
	dir := t.TempDir()
	// 97 valid files
	for i := 0; i < 97; i++ {
		writeTestJSON(t, dir, "valid-"+string(rune('a'+i%26))+"-"+string(rune('0'+i/26))+".json", `{
			"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
			"textContent": "Content", "title": "Note",
			"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
			"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
		}`)
	}
	// 3 corrupted files
	writeTestJSON(t, dir, "corrupt1.json", `{invalid json`)
	writeTestJSON(t, dir, "corrupt2.json", ``)
	writeTestJSON(t, dir, "corrupt3.json", `not json at all`)

	parser := NewTakeoutParser()
	notes, errors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if len(notes) != 97 {
		t.Errorf("notes = %d, want 97", len(notes))
	}
	if len(errors) != 3 {
		t.Errorf("parse errors = %d, want 3", len(errors))
	}
}

func TestNoteIDFromFilename(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{Title: "Test"}
	id := parser.NoteID(note, "/exports/keep/My Important Note.json")
	if id != "My Important Note" {
		t.Errorf("note ID = %q, want %q", id, "My Important Note")
	}
}

func TestModifiedAtConversion(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{
		UserEditedTimestampUsec: 1712000000000000, // microseconds
	}
	modifiedAt := parser.ModifiedAt(note)
	if modifiedAt.IsZero() {
		t.Fatal("modified_at should not be zero")
	}
	// 1712000000000000 usec = 1712000000 sec = ~April 1, 2024
	expected := time.UnixMicro(1712000000000000)
	if !modifiedAt.Equal(expected) {
		t.Errorf("modified_at = %v, want %v", modifiedAt, expected)
	}
}

func TestCursorFiltering(t *testing.T) {
	parser := NewTakeoutParser()

	baseTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	cursor := baseTime.Format(time.RFC3339)

	var notes []TakeoutNote
	// 197 notes before cursor
	for i := 0; i < 197; i++ {
		notes = append(notes, TakeoutNote{
			TextContent:             "Old note",
			UserEditedTimestampUsec: baseTime.Add(-time.Duration(i+1) * time.Hour).UnixMicro(),
			CreatedTimestampUsec:    baseTime.Add(-time.Duration(i+100) * time.Hour).UnixMicro(),
		})
	}
	// 3 notes after cursor
	for i := 0; i < 3; i++ {
		notes = append(notes, TakeoutNote{
			TextContent:             "New note",
			UserEditedTimestampUsec: baseTime.Add(time.Duration(i+1) * time.Hour).UnixMicro(),
			CreatedTimestampUsec:    baseTime.Add(-time.Duration(i) * time.Hour).UnixMicro(),
		})
	}

	filtered, newCursor := parser.FilterByCursor(notes, cursor)
	if len(filtered) != 3 {
		t.Errorf("filtered = %d, want 3", len(filtered))
	}
	if newCursor == cursor {
		t.Error("cursor should have advanced")
	}
}

func TestCorruptedJSONDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "truncated.json", `{"title": "Test", "textContent": `)
	writeTestJSON(t, dir, "empty.json", ``)
	writeTestJSON(t, dir, "binary.json", string([]byte{0x00, 0x01, 0xFF, 0xFE}))

	parser := NewTakeoutParser()
	notes, errors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("should not return error: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %d, want 0", len(notes))
	}
	if len(errors) != 3 {
		t.Errorf("errors = %d, want 3", len(errors))
	}
}

func TestParseExportPreservesSourceFile(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "unique-filename-abc.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Content", "title": "Duplicate Title",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	writeTestJSON(t, dir, "unique-filename-def.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Content", "title": "Duplicate Title",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	parser := NewTakeoutParser()
	notes, _, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %d, want 2", len(notes))
	}

	// Both notes have same Title but different SourceFile
	ids := make(map[string]bool)
	for _, n := range notes {
		if n.SourceFile == "" {
			t.Error("SourceFile should be populated by ParseExport")
		}
		id := parser.NoteID(&n, n.SourceFile)
		if ids[id] {
			t.Errorf("duplicate NoteID %q — SourceFile should prevent collisions", id)
		}
		ids[id] = true
	}
}

func TestSourceFilePreservedThroughCursorFilter(t *testing.T) {
	parser := NewTakeoutParser()

	notes := []TakeoutNote{
		{
			TextContent:             "Note 1",
			UserEditedTimestampUsec: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC).UnixMicro(),
			SourceFile:              "note-aaa.json",
		},
		{
			TextContent:             "Note 2",
			UserEditedTimestampUsec: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixMicro(),
			SourceFile:              "note-bbb.json",
		},
	}

	cursor := "2026-03-15T00:00:00Z"
	filtered, _ := parser.FilterByCursor(notes, cursor)
	if len(filtered) != 1 {
		t.Fatalf("filtered = %d, want 1", len(filtered))
	}
	if filtered[0].SourceFile != "note-aaa.json" {
		t.Errorf("SourceFile = %q, want note-aaa.json", filtered[0].SourceFile)
	}
}

// --- Security Tests ---

func TestParseExportRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	secretDir := t.TempDir()
	writeTestJSON(t, secretDir, "secret.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "SECRET DATA", "title": "Secret",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	// Create a symlink inside the export directory pointing to a file outside it
	symPath := filepath.Join(dir, "link-to-secret.json")
	if err := os.Symlink(filepath.Join(secretDir, "secret.json"), symPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// Also add a legitimate note to verify normal parsing still works
	writeTestJSON(t, dir, "legitimate.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Normal note", "title": "Legit",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	parser := NewTakeoutParser()
	notes, parseErrors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("ParseExport should not fail: %v", err)
	}

	// The symlink target should NOT be parsed — it must appear in the error list
	if len(notes) != 1 {
		t.Errorf("notes = %d, want 1 (legitimate note only)", len(notes))
	}
	if len(parseErrors) != 1 {
		t.Errorf("errors = %d, want 1 (symlink rejected)", len(parseErrors))
	}
	for _, n := range notes {
		if n.TextContent == "SECRET DATA" {
			t.Fatal("SECURITY: symlink to external file was parsed — directory traversal is possible")
		}
	}
}

func TestParseNoteFileSizeLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a file that exceeds maxNoteFileSize
	path := filepath.Join(dir, "huge.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Write just enough to trigger the size check (write a small header then truncate to size)
	if err := f.Truncate(maxNoteFileSize + 1); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()

	parser := NewTakeoutParser()
	_, err = parser.ParseNoteFile(path)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

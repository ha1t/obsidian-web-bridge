package mcpserver

import "testing"

func TestClassify(t *testing.T) {
	p := NewPolicy([]string{"DailyReports", "MorningBrief", "Calendar", "Garmin", "MonthlyReview", "Templates", "bases"})
	cases := map[string]FolderClass{
		"DailyNotes/2026-11-01.md": FolderDailyNotes,
		"dailynotes/x.md":          FolderDailyNotes,
		"DailyReports/x.md":        FolderReadonly,
		"dailyreports/x.md":        FolderReadonly,
		"Notes/x.md":               FolderWritable,
		"Clippings/x.md":           FolderWritable,
		"DailyReportsArchive/x.md": FolderWritable,
		"x.md":                     FolderWritable,
	}
	for in, want := range cases {
		if got := p.Classify(in); got != want {
			t.Errorf("Classify(%q)=%v want %v", in, got, want)
		}
	}
}

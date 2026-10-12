package hours

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"2", 8, false},
		{"2.75", 11, false},
		{"2,75", 11, false},
		{" 2,5 ", 10, false},
		{"2.50", 10, false},
		{"2.500", 10, false},
		{".25", 1, false},
		{"0.25", 1, false},
		{"0", 0, false},
		{"2:45", 11, false},
		{"0:15", 1, false},
		{"10:00", 40, false},
		{"2.", 8, false},

		{"", 0, true},
		{"abc", 0, true},
		{"0.3", 0, true},
		{"2.1", 0, true},
		{"2.333", 0, true},
		{"-1", 0, true},
		{"2:20", 0, true},
		{"2:60", 0, true},
		{"2:5", 0, true},
		{":30", 0, true},
		{"1e2", 0, true},
		{"+2", 0, true},
		{"2.5.5", 0, true},
		{".", 0, true},
		{"2 h", 0, true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("Parse(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		q    int
		want string
	}{
		{0, "0,00"},
		{1, "0,25"},
		{11, "2,75"},
		{20, "5,00"},
		{-3, "-0,75"},
	}
	for _, tt := range tests {
		if got := Format(tt.q); got != tt.want {
			t.Errorf("Format(%d) = %q, want %q", tt.q, got, tt.want)
		}
	}
	if got := FormatSep(10, "."); got != "2.50" {
		t.Errorf("FormatSep(10, \".\") = %q, want 2.50", got)
	}
}

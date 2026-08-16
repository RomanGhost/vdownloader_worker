package fileserver

import "testing"

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain title unchanged", "My Video Title", "My Video Title"},
		{"windows-forbidden characters replaced", `a/b\c:d*e?f"g<h>i|j`, "a_b_c_d_e_f_g_h_i_j"},
		{"control characters replaced", "a\x00b\x1fc", "a_b_c"},
		{"surrounding whitespace trimmed", "  padded title  ", "padded title"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeFilename(tc.input); got != tc.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

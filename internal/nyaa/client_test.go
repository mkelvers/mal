package nyaa

import "testing"

func TestParseEpisodeNumber(t *testing.T) {
	tests := []struct {
		title    string
		expected int
	}{
		// Standard fansub formats
		{"[SubsPlease] Naruto - 01 (1080p) [ABC123].mkv", 1},
		{"[SubsPlease] Naruto - 220 (1080p) [ABC123].mkv", 220},
		{"[Erai-raws] One Piece - 1100 [1080p][Multiple Subtitle].mkv", 1100},

		// S01E01 format
		{"Naruto S01E01 1080p WEB-DL", 1},
		{"Attack on Titan S04E28 Final", 28},

		// Episode keyword
		{"Naruto Episode 1 [1080p]", 1},
		{"One Piece Episode 1089", 1089},

		// E01 format
		{"Naruto E01 [1080p]", 1},
		{"Bleach E366 Final", 366},

		// Hash/number format
		{"Anime Title #42 [720p]", 42},

		// Should NOT match these (batches/complete)
		{"[SubsPlease] Naruto (01-220) [Batch]", 0},
		{"Naruto Complete Series 1-220", 0},
		{"Naruto Batch 01~220", 0},

		// Should NOT match resolutions/years as episodes
		{"Naruto The Movie 2024 [1080p]", 0},
		{"[1920x1080] Naruto Remastered", 0},

		// Version numbers should be stripped
		{"[SubsPlease] Naruto - 05v2 (1080p)", 5},
		{"Bleach - 100v3 [720p]", 100},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := parseEpisodeNumber(tt.title)
			if got != tt.expected {
				t.Errorf("parseEpisodeNumber(%q) = %d, want %d", tt.title, got, tt.expected)
			}
		})
	}
}

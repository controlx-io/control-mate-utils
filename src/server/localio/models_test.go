package localio

import "testing"

func TestGuessModel(t *testing.T) {
	tests := []struct {
		di, do, ai, ao int
		expected       string
	}{
		{4, 4, 0, 0, "IO4040"},
		{0, 4, 4, 0, "IO0440"},
		{0, 8, 0, 0, "IO0080"},
		{8, 0, 0, 0, "IO8000"},
		{0, 0, 4, 4, "IO0404"},
		{0, 0, 0, 0, "Unknown"},
		{1, 1, 1, 1, "Unknown"},
	}

	for _, tt := range tests {
		result := guessModel(tt.di, tt.do, tt.ai, tt.ao)
		if result != tt.expected {
			t.Errorf("guessModel(%d, %d, %d, %d) = %s; want %s",
				tt.di, tt.do, tt.ai, tt.ao, result, tt.expected)
		}
	}
}

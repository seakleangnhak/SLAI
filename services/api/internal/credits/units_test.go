package credits

import "testing"

func TestFromDecimalStringScalesDisplayedCredits(t *testing.T) {
	tests := []struct {
		value string
		want  int64
	}{
		{value: "1", want: 1_000_000},
		{value: "0.001", want: 1_000},
		{value: "0.000001", want: 1},
		{value: "0.0000001", want: 1},
		{value: "1.25", want: 1_250_000},
	}

	for _, test := range tests {
		got, err := FromDecimalString(test.value)
		if err != nil {
			t.Fatalf("FromDecimalString(%q) error: %v", test.value, err)
		}
		if got != test.want {
			t.Fatalf("FromDecimalString(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestFromDecimalStringRejectsInvalidValue(t *testing.T) {
	if _, err := FromDecimalString("not-a-number"); err == nil {
		t.Fatal("expected invalid decimal error")
	}
}

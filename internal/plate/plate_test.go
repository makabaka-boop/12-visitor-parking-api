package plate
import "testing"
func TestValid(t *testing.T) {
	cases := map[string]bool{
		"京A12345":   true,
		"沪B12345":   true,
		"粤BD12345":  true, // new energy 8-char
		"京A1234":    false,
		"":          false,
		"ABCDEF":    false,
		"京A123456":  true, // 8-char
		"京A1234567": false,
	}
	for in, want := range cases {
		got := Valid(in)
		if got != want {
			t.Errorf("Valid(%q)=%v want %v", in, got, want)
		}
	}
}
func TestNormalize(t *testing.T) {
	if got := Normalize(" 京a12345 "); got != "京A12345" {
		t.Errorf("Normalize=%q want 京A12345", got)
	}
}

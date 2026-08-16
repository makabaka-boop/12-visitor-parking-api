package plate
import "regexp"
var plateRe = regexp.MustCompile(
	`^[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤川青藏琼宁]` +
		`[A-Z]` +
		`[A-HJ-NP-Z0-9]{5,6}$`,
)
func Normalize(p string) string {
	out := make([]byte, 0, len(p))
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, byte(r-32))
		case r == ' ' || r == '\t':
		default:
			out = append(out, []byte(string(r))...)
		}
	}
	return string(out)
}
func Valid(p string) bool {
	return plateRe.MatchString(Normalize(p))
}

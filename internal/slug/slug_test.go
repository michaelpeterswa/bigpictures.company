package slug

import "testing"

func TestFromTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Mt Rainier from Crystal Lookout", "mt-rainier-from-crystal-lookout"},
		{"  Trailing  Spaces  ", "trailing-spaces"},
		{"Émojis 🎉 and Accents", "emojis-and-accents"},
		{"", ""},
	}
	for _, c := range cases {
		if got := FromTitle(c.in); got != c.want {
			t.Errorf("FromTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	good := []string{"ab", "mt-rainier", "test-pano-2025", "a1"}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("Validate(%q) failed: %v", s, err)
		}
	}
	bad := []string{
		"", "a", "-leading-dash", "trailing-dash-", "UPPERCASE", "has space", "weird_underscore",
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("Validate(%q) should have failed", s)
		}
	}
}

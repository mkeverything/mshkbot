package utils

import (
	"strings"
	"testing"
)

func TestRandomCongratulationContainsMention(t *testing.T) {
	mention := "[test](https://t.me/c/1/2)"
	for range 400 {
		out := RandomCongratulation(mention)
		if !strings.Contains(out, mention) {
			t.Fatalf("missing mention in %q", out)
		}
	}
}

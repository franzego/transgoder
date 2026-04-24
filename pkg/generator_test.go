package pkg

import (
	"regexp"
	"testing"
)

func TestGenerateID(t *testing.T) {
	id := GenerateID()
	if ok, _ := regexp.MatchString(`^JB-[A-F0-9]{8}$`, id); !ok {
		t.Fatalf("unexpected id format: %q", id)
	}
}

package pkg

import "testing"

func TestTextOrNull(t *testing.T) {
	got := TextOrNull("")
	if got.Valid {
		t.Fatalf("expected invalid text for empty input, got %+v", got)
	}

	got = TextOrNull("hello")
	if !got.Valid || got.String != "hello" {
		t.Fatalf("unexpected text value: %+v", got)
	}
}

func TestIntOrNull(t *testing.T) {
	got := IntOrNull(nil)
	if got.Valid {
		t.Fatalf("expected invalid int for nil input, got %+v", got)
	}

	v := int32(42)
	got = IntOrNull(&v)
	if !got.Valid || got.Int32 != 42 {
		t.Fatalf("unexpected int value: %+v", got)
	}
}

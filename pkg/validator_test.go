package pkg

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestRegisterCustomValidations_Timezone(t *testing.T) {
	v := validator.New()
	RegisterCustomValidations(v)

	type req struct {
		TZ string `validate:"timezone"`
	}

	if err := v.Struct(req{TZ: "Africa/Lagos"}); err != nil {
		t.Fatalf("expected valid timezone, got %v", err)
	}
	if err := v.Struct(req{TZ: "Not/AZone"}); err == nil {
		t.Fatal("expected invalid timezone error, got nil")
	}
}

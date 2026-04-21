package pkg

import (
	"time"

	"github.com/go-playground/validator/v10"
)

func RegisterCustomValidations(v *validator.Validate) {
	_ = v.RegisterValidation("timezone", func(fl validator.FieldLevel) bool {
		tz := fl.Field().String()
		_, err := time.LoadLocation(tz)
		return err == nil
	})
}

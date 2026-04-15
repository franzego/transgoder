package pkg

import (
	"strings"

	"github.com/google/uuid"
)

func GenerateID() string {
	id := uuid.New().String()
	short := strings.ReplaceAll(id, "-", "")[:8]
	return "JB-" + strings.ToUpper(short)
}

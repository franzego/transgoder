package pkg

import "github.com/jackc/pgx/v5/pgtype"

// This is important because we need to convert empty strings to null values when inserting into the database.
// The sqlc generated code expects pgtype.Text and pgtype.Int4 for nullable fields,
// so we need helper functions to convert our input to the correct types.

func TextOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func IntOrNull(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

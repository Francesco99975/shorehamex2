package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
)

type NullableString sql.NullString

// Implement json.Marshaler for NullableString
func (ns NullableString) MarshalJSON() ([]byte, error) {
	if ns.Valid {
		return json.Marshal(ns.String)
	}
	return json.Marshal(nil) // Return null if invalid
}

// Implement sql.Scanner for NullableString
func (ns *NullableString) Scan(value any) error {
	return (*sql.NullString)(ns).Scan(value)
}

// Implement driver.Valuer for NullableString
func (ns NullableString) Value() (driver.Value, error) {
	// Explicitly convert NullableString to sql.NullString
	nullStr := sql.NullString(ns) // Convert to sql.NullString
	return nullStr.Value()        // Call the Value method of sql.NullString
}

type NullableInt sql.NullInt64

// Implement json.Marshaler for NullableInt
func (ni NullableInt) MarshalJSON() ([]byte, error) {
	if ni.Valid {
		return json.Marshal(ni.Int64)
	}
	return json.Marshal(nil) // Return null if invalid
}

// Implement sql.Scanner for NullableInt
func (ni *NullableInt) Scan(value any) error {
	return (*sql.NullInt64)(ni).Scan(value)
}

// Implement driver.Valuer for NullableInt
func (ni NullableInt) Value() (driver.Value, error) {
	// Explicitly convert NullableInt to sql.NullInt64
	nullInt := sql.NullInt64(ni) // Convert to sql.NullInt64
	return nullInt.Value()       // Call the Value method of sql.NullInt64
}

type NullableTime sql.NullTime

func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if nt.Valid {
		return json.Marshal(nt.Time)
	}
	return json.Marshal(nil)
}

// Implement sql.Scanner for NullableTime
func (nt *NullableTime) Scan(value any) error {
	return (*sql.NullTime)(nt).Scan(value)
}

// Implement driver.Valuer for NullableTime
func (nt NullableTime) Value() (driver.Value, error) {
	// Explicitly convert NullableTime to sql.NullTime
	nullTime := sql.NullTime(nt) // Convert to sql.NullTime
	return nullTime.Value()      // Call the Value method of sql.NullTime
}

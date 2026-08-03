package postgres

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func dbTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
func domainTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
func dbOptionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
func dbOptionalVersion(value uint64) (*int64, error) {
	if value == 0 {
		return nil, nil
	}
	converted, err := dbVersion(value)
	if err != nil {
		return nil, err
	}
	return &converted, nil
}
func domainOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func domainOptionalVersion(value *int64) uint64 {
	if value == nil || *value <= 0 {
		return 0
	}
	return uint64(*value)
}
func dbVersion(value uint64) (int64, error) {
	if value == 0 || value > uint64(^uint64(0)>>1) {
		return 0, fmt.Errorf("version is outside PostgreSQL bigint range")
	}
	return int64(value), nil
}
func domainVersion(value int64) (uint64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("persisted version must be positive")
	}
	return uint64(value), nil
}

package ptr

// Int32 returns pointer to int32 value.
func Int32(v int32) *int32 {
	return &v
}

// Int32 returns pointer to int64 value.
func Int64(v int64) *int64 {
	return &v
}

// Bool returns pointer to bool value.
func Bool(v bool) *bool {
	return &v
}

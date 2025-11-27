package controllers

import (
	"net/url"
	"strconv"
)

const (
	// MaxPageLimit is the maximum number of items per page
	MaxPageLimit = 100

	// MaxContentLength is the maximum length for user-generated content
	MaxContentLength = 10000
)

// ParsePage extracts the page number from URL query params.
// Returns defaultPage if not present or invalid.
func ParsePage(query url.Values, defaultPage int) int {
	if pageStr := query.Get("page"); pageStr != "" {
		if val, err := strconv.Atoi(pageStr); err == nil && val > 0 {
			return val
		}
	}
	return defaultPage
}

// ParseLimit extracts the page limit from URL query params.
// Returns defaultLimit if not present or invalid, capped at MaxPageLimit.
func ParseLimit(query url.Values, defaultLimit int) int {
	limit := defaultLimit
	if limitStr := query.Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}
	return min(limit, MaxPageLimit)
}

package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Pagination is opt-in so existing dashboard consumers keep the array contract
// and complete dataset until they adopt paged requests.
func listPagination(c *gin.Context) (string, []any, bool) {
	limitText, hasLimit := c.GetQuery("limit")
	offsetText, hasOffset := c.GetQuery("offset")
	if !hasLimit && !hasOffset {
		return "", nil, true
	}
	limit, err := strconv.Atoi(limitText)
	if !hasLimit || err != nil || limit < 1 || limit > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 500; offset requires limit"})
		return "", nil, false
	}
	offset := 0
	if hasOffset {
		offset, err = strconv.Atoi(offsetText)
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
			return "", nil, false
		}
	}
	return " LIMIT ? OFFSET ?", []any{limit, offset}, true
}

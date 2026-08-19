package baize

const (
	defaultReadPageSize = 20
	maxReadPageSize     = 50
	maxReadTextLength   = 512
	maxReadSearchLength = 200
)

// ReadResultBoundary 统一说明只读查询的裁剪、脱敏和结果边界。
type ReadResultBoundary struct {
	ResultMode                       string `json:"resultMode"`
	SensitiveContentExcluded         bool   `json:"sensitiveContentExcluded"`
	UnknownSensitiveContentMayRemain bool   `json:"unknownSensitiveContentMayRemain"`
	RedactionApplied                 bool   `json:"redactionApplied"`
	RedactionPolicy                  string `json:"redactionPolicy"`
	Truncated                        bool   `json:"truncated"`
	Notice                           string `json:"notice"`
}

type ReadPageMeta struct {
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"pageSize"`
	HasMore  bool `json:"hasMore"`
	NextPage int  `json:"nextPage,omitempty"`
}

func newReadResultBoundary(notice string) ReadResultBoundary {
	return ReadResultBoundary{
		ResultMode:                       "bounded_read_query",
		SensitiveContentExcluded:         true,
		UnknownSensitiveContentMayRemain: true,
		RedactionPolicy:                  "conservative_patterns_only",
		Notice:                           trimPublicText(notice, 1000),
	}
}

func normalizeReadPage(page, pageSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = defaultReadPageSize
	}
	if page < 1 {
		return 0, 0, newInputError("page must be at least 1")
	}
	if pageSize < 1 || pageSize > maxReadPageSize {
		return 0, 0, newInputError("page size must be between 1 and 50")
	}
	return page, pageSize, nil
}

func makeReadPageMeta(total, page, pageSize int, itemsTruncated bool) ReadPageMeta {
	hasMore := itemsTruncated || page*pageSize < total
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return ReadPageMeta{Total: total, Page: page, PageSize: pageSize, HasMore: hasMore, NextPage: nextPage}
}

func validateReadFilter(value, label string, max int) (string, error) {
	value = trimPublicText(value, max+1)
	if len(value) > max {
		return "", newInputError(label + " is too long")
	}
	return value, nil
}

func redactBoundedReadText(value string, max int) (string, bool, bool) {
	value, truncated := trimPublicTextWithFlag(value, max)
	value, redacted := redactSensitiveTextUnbounded(value)
	return value, redacted, truncated
}

func isAllowedReadValue(value string, allowed map[string]struct{}) bool {
	if value == "" {
		return true
	}
	_, ok := allowed[value]
	return ok
}

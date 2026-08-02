package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const maxPageSelectionRanges = 10000

type pageInterval struct {
	start int
	end   int
}

// PageSelection describes the 1-based pages that should be sent to the model.
// An empty selection means every rendered page.
type PageSelection struct {
	intervals []pageInterval
	canonical string
}

// ParsePageSelection parses expressions such as "1,3-5,9". Page numbers are
// 1-based and intervals are inclusive.
func ParsePageSelection(value string) (PageSelection, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return PageSelection{}, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > maxPageSelectionRanges {
		return PageSelection{}, fmt.Errorf("page selection contains more than %d ranges", maxPageSelectionRanges)
	}
	intervals := make([]pageInterval, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return PageSelection{}, fmt.Errorf("page selection contains an empty range")
		}
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return PageSelection{}, fmt.Errorf("invalid page range %q", part)
		}
		start, err := parsePageNumber(bounds[0])
		if err != nil {
			return PageSelection{}, fmt.Errorf("invalid page range %q: %w", part, err)
		}
		end := start
		if len(bounds) == 2 {
			end, err = parsePageNumber(bounds[1])
			if err != nil {
				return PageSelection{}, fmt.Errorf("invalid page range %q: %w", part, err)
			}
			if start > end {
				return PageSelection{}, fmt.Errorf("page range %q starts after it ends", part)
			}
		}
		intervals = append(intervals, pageInterval{start: start, end: end})
	}
	sort.Slice(intervals, func(left int, right int) bool {
		if intervals[left].start == intervals[right].start {
			return intervals[left].end < intervals[right].end
		}
		return intervals[left].start < intervals[right].start
	})
	intervals = mergePageIntervals(intervals)
	return PageSelection{intervals: intervals, canonical: formatPageIntervals(intervals)}, nil
}

// NormalizePageSelection validates a selection and returns its canonical form.
func NormalizePageSelection(value string) (string, error) {
	selection, err := ParsePageSelection(value)
	if err != nil {
		return "", err
	}
	return selection.canonical, nil
}

func (selection PageSelection) String() string {
	return selection.canonical
}

// Pages expands a non-empty selection against a known source page count.
func (selection PageSelection) Pages(sourcePageCount int) ([]int, error) {
	if sourcePageCount <= 0 {
		return nil, fmt.Errorf("source page count must be positive")
	}
	if len(selection.intervals) == 0 {
		return nil, nil
	}
	for _, interval := range selection.intervals {
		if interval.end > sourcePageCount {
			return nil, fmt.Errorf("requested page range %s exceeds the rendered document's %d pages", formatPageInterval(interval), sourcePageCount)
		}
	}
	pages := make([]int, 0)
	for _, interval := range selection.intervals {
		for page := interval.start; ; page++ {
			pages = append(pages, page)
			if page == interval.end {
				break
			}
		}
	}
	return pages, nil
}

func (selection PageSelection) filter(pages []PageImage) ([]PageImage, error) {
	if len(selection.intervals) == 0 {
		return pages, nil
	}
	maxPage := 0
	for _, page := range pages {
		if page.Page > maxPage {
			maxPage = page.Page
		}
	}
	for _, interval := range selection.intervals {
		if interval.end > maxPage {
			return nil, fmt.Errorf("requested page range %s exceeds the rendered document's %d pages", formatPageInterval(interval), maxPage)
		}
	}
	selected := make([]PageImage, 0, len(pages))
	for _, page := range pages {
		if selection.includes(page.Page) {
			selected = append(selected, page)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("page selection %q did not match any rendered page", selection.canonical)
	}
	return selected, nil
}

func (selection PageSelection) includes(page int) bool {
	for _, interval := range selection.intervals {
		if page < interval.start {
			return false
		}
		if page <= interval.end {
			return true
		}
	}
	return false
}

func parsePageNumber(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("page number is empty")
	}
	page, err := strconv.Atoi(value)
	if err != nil || page <= 0 {
		return 0, fmt.Errorf("page number must be a positive integer")
	}
	return page, nil
}

func mergePageIntervals(intervals []pageInterval) []pageInterval {
	merged := make([]pageInterval, 0, len(intervals))
	for _, interval := range intervals {
		if len(merged) == 0 || interval.start > merged[len(merged)-1].end+1 {
			merged = append(merged, interval)
			continue
		}
		if interval.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = interval.end
		}
	}
	return merged
}

func formatPageIntervals(intervals []pageInterval) string {
	parts := make([]string, 0, len(intervals))
	for _, interval := range intervals {
		parts = append(parts, formatPageInterval(interval))
	}
	return strings.Join(parts, ",")
}

func formatPageInterval(interval pageInterval) string {
	if interval.start == interval.end {
		return strconv.Itoa(interval.start)
	}
	return strconv.Itoa(interval.start) + "-" + strconv.Itoa(interval.end)
}

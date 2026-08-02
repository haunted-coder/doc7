package extract

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/magicrew/doc7/internal/render"
	"github.com/magicrew/doc7/internal/vlm"
)

const maxNumericCorrectionTokens = 8

var numericTokenPattern = regexp.MustCompile(`(?:\([0-9][0-9,]*(?:\.[0-9]+)?%?\)|[+\-−－△]?[0-9][0-9,]*(?:\.[0-9]+)?%?)`)

type numericMismatch struct {
	Position int
	Source   string
	Current  string
}

type indexedNumericToken struct {
	Position int
	Value    string
}

func findNumericMismatches(source string, content string) []numericMismatch {
	sourceOnly := unmatchedNumericTokens(criticalNumericTokens(source), criticalNumericTokens(content))
	contentOnly := unmatchedNumericTokens(criticalNumericTokens(content), criticalNumericTokens(source))
	if len(sourceOnly) == 0 || len(sourceOnly) != len(contentOnly) {
		return nil
	}
	mismatches := make([]numericMismatch, 0, len(sourceOnly))
	for index := range sourceOnly {
		if isMathLine(numericTokenLine(content, contentOnly[index].Position)) {
			continue
		}
		mismatches = append(mismatches, numericMismatch{
			Position: contentOnly[index].Position,
			Source:   sourceOnly[index].Value,
			Current:  contentOnly[index].Value,
		})
	}
	return mismatches
}

func isMathLine(value string) bool {
	return strings.ContainsAny(value, "$^{}\\")
}

func unmatchedNumericTokens(tokens []string, other []string) []indexedNumericToken {
	matched := numericTokenCounts(other)
	result := make([]indexedNumericToken, 0)
	for index, token := range tokens {
		normalized := normalizeNumericToken(token)
		if matched[normalized] > 0 {
			matched[normalized]--
			continue
		}
		result = append(result, indexedNumericToken{Position: index + 1, Value: token})
	}
	return result
}

func numericSequencesEqual(source string, content string) bool {
	left := numericTokenCounts(criticalNumericTokens(source))
	right := numericTokenCounts(criticalNumericTokens(content))
	if len(left) != len(right) {
		return false
	}
	for token, count := range left {
		if right[token] != count {
			return false
		}
	}
	return true
}

func numericTokenCounts(tokens []string) map[string]int {
	counts := make(map[string]int, len(tokens))
	for _, token := range tokens {
		counts[normalizeNumericToken(token)]++
	}
	return counts
}

func normalizedNumericTokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range criticalNumericTokens(value) {
		result[normalizeNumericToken(token)] = struct{}{}
	}
	return result
}

func criticalNumericTokens(value string) []string {
	indices := numericTokenPattern.FindAllStringIndex(value, -1)
	tokens := make([]string, 0, len(indices))
	for _, span := range indices {
		match := value[span[0]:span[1]]
		if prefixBytes := numericJoinerPrefixBytes(value, span[0], match); prefixBytes > 0 {
			match = match[prefixBytes:]
		}
		if isCriticalNumericToken(match) {
			tokens = append(tokens, match)
		}
	}
	return tokens
}

func numericJoinerPrefixBytes(value string, start int, match string) int {
	if start == 0 || len(match) == 0 {
		return 0
	}
	sign, signBytes := utf8.DecodeRuneInString(match)
	if !strings.ContainsRune("-−－", sign) {
		return 0
	}
	previous, _ := utf8.DecodeLastRuneInString(value[:start])
	if (previous >= '0' && previous <= '9') || (previous >= 'A' && previous <= 'Z') || (previous >= 'a' && previous <= 'z') {
		return signBytes
	}
	return 0
}

func isCriticalNumericToken(value string) bool {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "()")
	digits := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits++
		}
	}
	return digits >= 3 || strings.ContainsAny(value, ".%$€£¥")
}

func normalizeNumericToken(value string) string {
	value = strings.TrimSuffix(strings.TrimSpace(value), ",")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "−", "-")
	value = strings.ReplaceAll(value, "－", "-")
	if strings.HasPrefix(value, "△") {
		value = "-" + strings.TrimPrefix(value, "△")
	}
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "("), ")")
	}
	return value
}

func numericMismatchTokens(mismatches []numericMismatch) []string {
	result := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		result = append(result, mismatch.Source)
	}
	return result
}

func correctNumericMismatches(ctx context.Context, client vlm.Client, page render.PageImage, content string, mismatches []numericMismatch, detail string, retryCount int) (string, vlm.Usage, error) {
	confirmed := make(map[int]bool, len(mismatches))
	var usage vlm.Usage
	for _, mismatch := range mismatches {
		response, err := completeWithRetry(ctx, client, vlm.Request{
			Prompt:      numericGroundingPrompt(content, mismatch),
			ImagePath:   page.ImagePath,
			ImageMIME:   mime.TypeByExtension(filepath.Ext(page.ImagePath)),
			ImageDetail: detail,
		}, retryCount)
		usage = addUsage(usage, response.Usage)
		if err != nil {
			return "", usage, err
		}
		ok, err := parseNumericConfirmation(response.Content, mismatch.Source)
		if err != nil {
			return "", usage, err
		}
		confirmed[mismatch.Position] = ok
	}
	corrected, err := applyNumericCorrections(content, mismatches, confirmed)
	if err != nil {
		return "", usage, err
	}
	return corrected, usage, nil
}

func numericGroundingPrompt(content string, mismatch numericMismatch) string {
	var builder strings.Builder
	builder.WriteString(`Read the page image and transcribe the exact numeric value in the same cell as the current token below. The Markdown row is included only to identify the row and column. Return only the value printed in the image, preserving its sign, punctuation, and percent symbol.

`)
	fmt.Fprintf(&builder, "Current token: `%s`\nCandidate from source text: `%s`\nRow context: `%s`", mismatch.Current, mismatch.Source, numericTokenLine(content, mismatch.Position))
	return builder.String()
}

func parseNumericConfirmation(value string, expected string) (bool, error) {
	tokens := criticalNumericTokens(value)
	if len(tokens) != 1 {
		return false, vlm.NewError(vlm.ParseError, "numeric grounding response did not contain exactly one value", false, nil)
	}
	return normalizeNumericToken(tokens[0]) == normalizeNumericToken(expected), nil
}

func numericTokenLine(content string, position int) string {
	spans := criticalNumericSpans(content)
	if position <= 0 || position > len(spans) {
		return ""
	}
	start := spans[position-1][0]
	lineStart := strings.LastIndex(content[:start], "\n") + 1
	lineEnd := strings.IndexByte(content[start:], '\n')
	if lineEnd < 0 {
		lineEnd = len(content)
	} else {
		lineEnd += start
	}
	return strings.TrimSpace(content[lineStart:lineEnd])
}

func applyNumericCorrections(content string, mismatches []numericMismatch, confirmations map[int]bool) (string, error) {
	spans := criticalNumericSpans(content)
	for _, mismatch := range mismatches {
		if !confirmations[mismatch.Position] {
			return "", vlm.NewError(vlm.ParseError, "model rejected a numeric grounding correction", false, nil)
		}
		if mismatch.Position <= 0 || mismatch.Position > len(spans) {
			return "", vlm.NewError(vlm.ParseError, "numeric grounding position is outside Markdown", false, nil)
		}
		current := content[spans[mismatch.Position-1][0]:spans[mismatch.Position-1][1]]
		if normalizeNumericToken(current) != normalizeNumericToken(mismatch.Current) {
			return "", vlm.NewError(vlm.ParseError, "numeric grounding current token changed before replacement", false, nil)
		}
	}
	corrected := content
	for index := len(mismatches) - 1; index >= 0; index-- {
		mismatch := mismatches[index]
		span := spans[mismatch.Position-1]
		corrected = corrected[:span[0]] + mismatch.Source + corrected[span[1]:]
	}
	return corrected, nil
}

func criticalNumericSpans(value string) [][2]int {
	spans := make([][2]int, 0)
	for _, span := range numericTokenPattern.FindAllStringIndex(value, -1) {
		start := span[0]
		match := value[start:span[1]]
		if prefixBytes := numericJoinerPrefixBytes(value, start, match); prefixBytes > 0 {
			start += prefixBytes
		}
		if isCriticalNumericToken(value[start:span[1]]) {
			spans = append(spans, [2]int{start, span[1]})
		}
	}
	return spans
}

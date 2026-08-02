package extract

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
)

func cacheKey(imageHash string, promptHash string, provider string, baseURL string, model string, imageDetail string, dpi int, maxTokens int, maxImageBytes int64, contextFallbacks int, minImageDimension int) string {
	parts := []string{
		"doc7-cache-v8",
		imageHash,
		promptHash,
		provider,
		baseURL,
		model,
		imageDetail,
		strconvItoa(dpi),
		strconvItoa(maxTokens),
		strconv.FormatInt(maxImageBytes, 10),
		strconvItoa(contextFallbacks),
		strconvItoa(minImageDimension),
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(hash[:])
}

func VisualCacheKey(imageHash string, promptHash string, provider string, baseURL string, model string, imageDetail string, dpi int, maxTokens int, maxImageBytes int64, contextFallbacks int, minImageDimension int) string {
	return cacheKey(imageHash, promptHash, provider, baseURL, model, imageDetail, dpi, maxTokens, maxImageBytes, contextFallbacks, minImageDimension)
}

func sha256Text(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

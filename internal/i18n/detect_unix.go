//go:build !darwin && !windows

package i18n

func detectSystemLocale() string {
	return ""
}

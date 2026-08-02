//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

func detectSystemLocale() string {
	output, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(output))
	value = strings.Trim(value, "()\n\r\t ")
	if first, _, ok := strings.Cut(value, ","); ok {
		value = first
	}
	return strings.Trim(value, "\"'\n\r\t ")
}

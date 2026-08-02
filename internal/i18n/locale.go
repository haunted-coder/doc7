package i18n

import (
	"fmt"
	"os"
	"strings"
)

type Language string

const (
	LanguageAuto      Language = "auto"
	LanguageEnglish   Language = "en"
	LanguageChineseCN Language = "zh-CN"
)

type Localizer struct {
	language Language
}

func New(preference string) Localizer {
	language := Normalize(preference)
	if language == LanguageAuto {
		language = DetectSystem()
	}
	return Localizer{language: language}
}

func Normalize(value string) Language {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "_", "-")))
	switch {
	case normalized == "", normalized == "auto":
		return LanguageAuto
	case normalized == "en", strings.HasPrefix(normalized, "en-"):
		return LanguageEnglish
	case normalized == "zh", strings.HasPrefix(normalized, "zh-"):
		return LanguageChineseCN
	default:
		return LanguageEnglish
	}
}

func DetectSystem() Language {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			if language := Normalize(value); language != LanguageEnglish || strings.HasPrefix(strings.ToLower(value), "en") {
				return language
			}
		}
	}
	if value := detectSystemLocale(); value != "" {
		return Normalize(value)
	}
	return LanguageEnglish
}

func (l Localizer) Language() Language {
	return l.language
}

func (l Localizer) Chinese() bool {
	return l.language == LanguageChineseCN
}

func (l Localizer) T(key string, args ...interface{}) string {
	template := englishMessages[key]
	if l.Chinese() {
		if translated := chineseMessages[key]; translated != "" {
			template = translated
		}
	}
	if template == "" {
		template = key
	}
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}

package text

// LanguageNames maps ISO 639-1 language codes to human-readable names.
var LanguageNames = map[string]string{
	"ru": "Russian",
	"en": "English",
	"de": "German",
	"fr": "French",
	"es": "Spanish",
	"it": "Italian",
	"pt": "Portuguese",
	"zh": "Chinese",
	"ja": "Japanese",
	"ko": "Korean",
	"ar": "Arabic",
	"hi": "Hindi",
	"nl": "Dutch",
	"pl": "Polish",
	"tr": "Turkish",
	"vi": "Vietnamese",
}

// SupportedSourceLanguages returns a map of language codes to names
// that can be used as source languages for transcription and translation.
var SupportedSourceLanguages = map[string]string{
	"ru": "Russian",
	"en": "English",
	"de": "German",
	"fr": "French",
	"es": "Spanish",
	"it": "Italian",
	"pt": "Portuguese",
	"nl": "Dutch",
	"pl": "Polish",
	"ja": "Japanese",
	"zh": "Chinese",
	"ko": "Korean",
	"ar": "Arabic",
	"hi": "Hindi",
}

// SupportedTargetLanguages returns a map of language codes to names
// that can be used as target languages for translation and TTS.
var SupportedTargetLanguages = map[string]string{
	"en": "English",
	"de": "German",
	"fr": "French",
	"es": "Spanish",
	"it": "Italian",
	"pt": "Portuguese",
	"nl": "Dutch",
	"pl": "Polish",
	"ja": "Japanese",
	"zh": "Chinese",
	"ru": "Russian",
}

// GetLanguageName returns the human-readable name for a language code.
// If the code is not found, it returns the code itself.
func GetLanguageName(code string) string {
	if name, ok := LanguageNames[code]; ok {
		return name
	}
	return code
}

// IsValidSourceLanguage checks if a language code is a valid source language.
func IsValidSourceLanguage(code string) bool {
	_, ok := SupportedSourceLanguages[code]
	return ok
}

// IsValidTargetLanguage checks if a language code is a valid target language.
func IsValidTargetLanguage(code string) bool {
	_, ok := SupportedTargetLanguages[code]
	return ok
}

// GetSourceLanguageCodes returns all valid source language codes.
func GetSourceLanguageCodes() []string {
	codes := make([]string, 0, len(SupportedSourceLanguages))
	for code := range SupportedSourceLanguages {
		codes = append(codes, code)
	}
	return codes
}

// GetTargetLanguageCodes returns all valid target language codes.
func GetTargetLanguageCodes() []string {
	codes := make([]string, 0, len(SupportedTargetLanguages))
	for code := range SupportedTargetLanguages {
		codes = append(codes, code)
	}
	return codes
}

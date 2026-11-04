package ir

// W-29 owns the module/stub registry and build profiles. These exact oracle
// cases require unimplemented functions or date/tokenizer/standard classes.
// They remain measured gaps; no class-declaration rule is excluded by prefix.
const knownGapCount = 12

var fileGaps = map[string]string{
	"0f751b921ccaaa2f.php": "Cannot use result of built-in function in write context",
	"8394ddee66cb947e.php": "Cannot use result of built-in function in write context",
	"84e338ba0be450ac.php": "Cannot use result of built-in function in write context",
	"57077f922869ea78.php": "Cannot redeclare function strlen()",
	"112d5af5f3ed8f0d.php": "Cannot override final method PhpToken::__construct()",
	"402a9cc0107f33e5.php": "Class Demo cannot extend enum RoundingMode",
	"4349b0fdb0c04751.php": "Declaration of MyDateTime::setTime(int $hour, int $minute, int $second = 0, bool $microsecond = false): DateTime must be compatible with DateTime::setTime(int $hour, int $minute, int $second = 0, int $microsecond = 0): DateTime",
	"4d6ab9ecad438856.php": "Declaration of MyDateTimeZone::listIdentifiers(): array must be compatible with DateTimeZone::listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null): array",
	"701e96e6335ce178.php": "Declaration of MyDateTimeZone::getTransitions(): array|false must be compatible with DateTimeZone::getTransitions(int $timestampBegin = PHP_INT_MIN, int $timestampEnd = 2147483647): array|false",
	"e5d4deb40296af22.php": "Declaration of MyDateTime::createFromFormat(): DateTime|false must be compatible with DateTime::createFromFormat(string $format, string $datetime, ?DateTimeZone $timezone = null): DateTime|false",
}

var warningGaps = map[string]string{
	"bc511790d900ab14.php": "Deprecated: Return type of MyDateTimeZone::listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null): string should either be compatible with DateTimeZone::listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null): array, or the #[\\ReturnTypeWillChange] attribute should be used to temporarily suppress the notice on line 4",
	"c9cb83075feeb650.php": "Deprecated: Return type of MyDateTimeZone::listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null) should either be compatible with DateTimeZone::listIdentifiers(int $timezoneGroup = DateTimeZone::ALL, ?string $countryCode = null): array, or the #[\\ReturnTypeWillChange] attribute should be used to temporarily suppress the notice on line 4",
}

func gapPrefix(file, msg string) string {
	if want, ok := fileGaps[file]; ok && msg == want {
		return "built-in declarations (W-29)"
	}
	return ""
}

func diagnosticGap(file string, want []string) string {
	if len(want) == 1 && warningGaps[file] == want[0] {
		return "built-in declarations (W-29)"
	}
	return ""
}

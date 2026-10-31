package frontend

import "testing"

func TestHeredocInterpolationNestedStringForms(t *testing.T) {
	for _, source := range []string{
		"<?php $s = <<<E\n{$a[`\"`]}\nE;",
		"<?php $s = <<<E\n{$a[<<<X\n\"\nX]}\nE;",
		"<?php $s = <<<E\n{$a[<<<'X'\n\"\nX]}\nE;",
		"<?php $s = <<<E\n{$a[\"{$b[\"}\"]}\"]}\nE;",
	} {
		if _, err := parse(t, source); err != nil {
			t.Errorf("PHP accepts %q: %v", source, err)
		}
	}
}

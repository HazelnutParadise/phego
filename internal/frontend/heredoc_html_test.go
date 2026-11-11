package frontend

import "testing"

func TestHeredocInterpolationPreservesPHPTagState(t *testing.T) {
	for _, src := range []string{
		"<?php $s = <<<E\n{$a[(function(){ ?>abc<?php return \"x\"; })()]}\nE;",
		"<?php $s = <<<E\n{$a[(function(){ ?> } \" { <?php return \"x\"; })()]}\nE;",
		"<?php $s = <<<E\n{$a[(function(){ // ?> abc <?php return \"x\"; })()]}\nE;",
		"<?php $s = <<<E\n{$a[(function(){ # ?> abc <?php return \"x\"; })()]}\nE;",
	} {
		if _, err := parse(t, src); err != nil {
			t.Errorf("PHP accepts %q: %v", src, err)
		}
	}
}

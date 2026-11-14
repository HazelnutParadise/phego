<?php
// ini: error_reporting=0
echo $u1, "a\n";
echo @$u2, "b\n";
$s = "abc";
echo $s[1.5], "c\n";
echo "x" . [], "d\n";
echo @("5 apples" + 1), "e\n";

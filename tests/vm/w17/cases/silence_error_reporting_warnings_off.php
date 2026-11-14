<?php
// ini: error_reporting=30717
echo $u1, "a\n";
echo @$u2, "b\n";
$s = "abc";
echo $s[1.5], "c\n";
echo @$s[2.5], "d\n";
$f = false;
$f[] = 1;
echo "e\n";
@($g[] = 1);
$g2 = false;
$g2[0] = 2;
echo "f\n";
echo @("x" . []), "g\n";
echo "x" . [], "h\n";

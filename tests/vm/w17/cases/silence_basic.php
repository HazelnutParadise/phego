<?php
echo @$u1, "a\n";
echo @@$u2, "b\n";
function w($x)
{
    echo $wu;
    return $x;
}
echo @w(@w(1)), "c\n";
echo @(w(2) . $u3), "d\n";
$x = @[1, 2][5];
echo "e\n";
echo @("5 apples" + 1), "f\n";
echo @$arr["k"], @$arr2["k"]["j"], "g\n";
$s = "abc";
echo @$s[10], "|", @$s["1x"], "h\n";
echo @(1 + [] . "x"), "i\n";
echo $u4, "j\n";

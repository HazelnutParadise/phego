<?php
function t($label)
{
    echo "<", $label, ">";
    return $label;
}
function three($a, $b, $c)
{
    echo "(body)";
    return $a . $b . $c;
}
echo "\n", three(t("a"), t("b"), t("c")), "\n";
echo three(t("x"), three(t("1"), t("2"), t("3")), t("y")), "\n";
$i = 1;
echo three($i, $i++, $i), "\n";
$j = 1;
echo three($j, ++$j, $j), "\n";
$k = 5;
$r = $k + $k++ + $k;
echo $r, "\n";
$arr = [];
$n = 0;
$arr[$n++] = $n;
echo $arr[0], " ", $n, "\n";
echo t("left") . t("right"), "\n";

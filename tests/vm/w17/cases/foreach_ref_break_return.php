<?php
function first(&$arr)
{
    foreach ($arr as $k => &$v) {
        if ($v > 1) {
            $v = "found";
            return $k;
        }
    }
    return -1;
}
$a = [1, 2, 3];
echo first($a), " ", $a[1], "\n";
$b = [1, 2, 3];
foreach ($b as &$v) {
    if ($v == 2) {
        break;
    }
}
$v = "x";
echo $b[0], $b[1], $b[2], "\n";
for ($i = 0; $i < 3; $i++) {
    foreach ($b as &$w) {
        continue 2;
    }
}
echo "done\n";

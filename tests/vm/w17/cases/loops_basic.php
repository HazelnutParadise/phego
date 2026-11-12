<?php
$i = 0;
while ($i < 3) {
    echo "w", $i;
    $i++;
}
echo "\n";
$j = 10;
do {
    echo "d", $j;
    $j++;
} while ($j < 3);
echo "\n";
for ($k = 0; $k < 5; $k += 2) {
    echo "f", $k;
}
echo "\n";
for ($a = 0, $b = 10; $a < $b; $a += 3, $b -= 3) {
    echo $a, "-", $b, " ";
}
echo "\n";
for (;;) {
    echo "once";
    break;
}
echo "\n";
$n = 3;
while ($n--)
    echo $n;
echo "\n";
for ($m = 0; $m < 2; $m++)
    echo "nobrace", $m;
echo "\n";
echo $k, " ", $n, "\n";

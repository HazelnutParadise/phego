<?php
$s = "hello";
echo $s[-1], "\n";
echo $s[-2], "\n";
echo $s[-5], "\n";
$i = 3;
echo $s[-$i], "\n";
echo $s[-1] . $s[-5], "\n";
$one = "z";
echo $one[-1], $one[0], "\n";
for ($k = 1; $k <= 5; $k++) {
    echo $s[-$k];
}
echo "\n";
echo $s[0 - 1], $s[1 - 3], "\n";

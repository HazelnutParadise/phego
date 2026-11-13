<?php
$a = [1, [2]];
$b = &$a[1][0];
$b = 3;
echo $a[1][0], "\n";
echo "ok\n";

<?php
$n = 5;
echo "before\n";
[$a, &$b] = $n;
echo "after\n";

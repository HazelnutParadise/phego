<?php
foreach ([1, 2] as $a) { foreach ([3, 4] as $v) { try { echo $a, $v, "\n"; break 2; } catch (Exception $e) { echo "c\n"; } } }
echo "after\n";

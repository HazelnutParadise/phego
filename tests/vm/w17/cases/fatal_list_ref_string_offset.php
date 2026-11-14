<?php
$s = "abc";
echo "before\n";
[&$a] = $s;
echo "after\n";

<?php
$a = [1];
echo "before\n";
unset($a[0][1]);
echo "not reached\n";

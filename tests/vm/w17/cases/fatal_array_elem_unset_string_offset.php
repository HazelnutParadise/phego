<?php
$s = "abc";
echo "before\n";
unset($s[0]);
echo "not reached\n";

<?php
$s = "abc";
echo "start\n";
$r = &$s[0];
echo "unreached\n";

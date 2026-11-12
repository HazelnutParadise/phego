<?php
$s = "hello";
echo $s[true], "\n";
echo $s[false], "\n";
echo $s[null], "\n";
$t = true;
echo $s[$t], "\n";
$u = null;
echo $s[$u], "\n";
echo $s[$undefined], "\n";

<?php
// String concatenation, substr, strrev and str_replace.
$s = '';
for ($i = 0; $i < 100000; $i++) { $s .= chr(97 + ($i * 7) % 26); }
$t = strrev($s);
$u = str_replace('abc', 'XYZ', $s . $t);
$sum = 0;
for ($i = 0; $i < strlen($u); $i += 101) { $sum = ($sum * 31 + ord($u[$i])) % 1000003; }
echo strlen($u), " ", substr($u, 1000, 12), " ", $sum, "\n";

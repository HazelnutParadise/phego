<?php
// PCRE matching and replacement on generated input.
$words = array();
$x = 3;
for ($i = 0; $i < 20000; $i++) { $x = ($x * 75 + 74) % 65537; $words[] = 'id' . $x . '@host' . ($x % 97) . '.test'; }
$text = implode(' ', $words);
$n = preg_match_all('/id(\d+)@host(\d{1,2})\.test/', $text, $m);
$r = preg_replace('/host(\d)\./', 'h$1.', $text);
$sum = 0;
for ($i = 0; $i < $n; $i += 37) { $sum = ($sum * 31 + (int)$m[1][$i]) % 1000003; }
echo $n, " ", strlen($r), " ", $sum, "\n";

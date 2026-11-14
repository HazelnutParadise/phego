<?php
[$a, $b] = [1];
echo $a, $b === null ? "null" : "?", "\n";
['k' => $c, 'j' => $d] = ['j' => 2];
echo $c === null ? "null" : "?", $d, "\n";
[$e, $f, $g] = [];
echo $e === null && $f === null && $g === null ? "all null" : "?", "\n";
[, , $h] = [1, 2];
echo $h === null ? "null" : "?", "\n";
[[$i, $j]] = [[1]];
echo $i, $j === null ? "null" : "?", "\n";
[$k[0], $k[1]] = [1];
echo $k[0], $k[1] === null ? "null" : "?", "\n";
[5 => $l] = [1, 2];
echo $l === null ? "null" : "?", "\n";
["0" => $m, "1" => $n] = ["a"];
echo $m, $n === null ? "null" : "?", "\n";

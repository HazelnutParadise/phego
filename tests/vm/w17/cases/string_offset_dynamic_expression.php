<?php
$s = "abcdefgh";
$i = 1;
echo $s[$i + 1], $s[$i * 3], $s[$i - 1], $s[7 - $i], "\n";
$idx = [0, 3, 6];
echo $s[$idx[1]], $s[$idx[2]], "\n";
echo $s[(int) "2"], $s[(int) 4.9], $s[(int) true], "\n";
echo $s[$i++], $s[$i++], $s[$i++], $i, "\n";
function get_pos()
{
    echo "pos ";
    return 5;
}
echo $s[get_pos()], "\n";
$j = 0;
echo $s[$j] . $s[$j = 4] . $s[$j], "\n";
$k = 2;
echo $s[$k ? 1 : 6], $s[$k > 5 ? 1 : 6], "\n";
echo $s[$k % 3], $s[$k ** 2], "\n";
echo $s[$k <=> 5], $s[-$k <=> 1], "\n";
echo $s[isset($s[8]) ? 8 : 0], "\n";

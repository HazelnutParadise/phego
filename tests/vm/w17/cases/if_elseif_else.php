<?php
function classify($n)
{
    if ($n < 0) {
        return "negative";
    } elseif ($n == 0) {
        return "zero";
    } else if ($n < 10) {
        return "small";
    } else {
        return "large";
    }
}
echo classify(-3), " ", classify(0), " ", classify(5), " ", classify(50), "\n";
if ("0") {
    echo "string zero true\n";
} else {
    echo "string zero false\n";
}
if ("0.0") echo "string 0.0 true\n";
if ([]) echo "never\n"; else echo "empty array false\n";
$x = 3;
if ($x > 1)
    echo "no braces\n";
elseif ($x > 0)
    echo "never\n";
else
    echo "never\n";
if (null) {
} elseif (0.0) {
} elseif ("a") {
    echo "third branch\n";
}

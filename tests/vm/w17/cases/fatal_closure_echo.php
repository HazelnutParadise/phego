<?php
// Runtime rule: a Closure object has no string form, so echo cannot print it.
// The lines before the bad echo show that the closure itself works.

function greet($name)
{
    return "hi " . $name;
}

$g = function ($name) {
    return "hello " . $name;
};

echo "call result: ", $g("ann"), "\n";
echo "named function result: ", greet("bob"), "\n";

$pairs = [1, 2, 3];
foreach ($pairs as $p) {
    echo "pair value: ", $p, "\n";
}

$sum = 0;
for ($i = 0; $i < 3; $i++) {
    $sum += $i;
}
echo "sum: ", $sum, "\n";

echo "about to echo a closure object\n";

echo $g;

echo "not reached\n";

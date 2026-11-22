<?php
// throw_in_loops: throws inside for, while, do-while, foreach by value, foreach by reference and object iteration
function maybe($i) {
    if ($i == 2) {
        throw new Exception("at $i");
    }
    return $i;
}
class Bag {
    public $a = 1;
    public $b = 2;
    public $c = 3;
}
$sum = 0;
try {
    for ($i = 0; $i < 5; $i++) {
        $sum = $sum + maybe($i);
    }
} catch (Exception $e) {
    echo "for_outside: ", $e->getMessage(), " sum=", $sum, "\n";
}

$hits = 0;
for ($j = 0; $j < 5; $j++) {
    try {
        maybe($j);
    } catch (Exception $e) {
        $hits = $hits + 1;
        continue;
    }
    echo "for_inside_ok: ", $j, "\n";
}
echo "for_inside_hits: ", $hits, "\n";
$k = 0;
try {
    while ($k < 5) {
        maybe($k);
        $k++;
    }
} catch (Exception $e) {
    echo "while_outside: k=", $k, "\n";
}

$m = 0;
do {
    try {
        maybe($m);
    } catch (Exception $e) {
        echo "do_inside_caught at ", $m, "\n";
    }
    $m++;
} while ($m < 4);
echo "do_end_m: ", $m, "\n";

$vals = [0, 1, 2, 3];
try {
    foreach ($vals as $v) {
        maybe($v);
        echo "fv: ", $v, "\n";
    }
} catch (Exception $e) {
    echo "foreach_value_outside: ", $e->getMessage(), "\n";
}

$refs = [10, 20, 30];
try {
    foreach ($refs as &$r) {
        $r = $r + 1;
        if ($r == 21) {
            throw new Exception("ref at 21");
        }
    }
} catch (Exception $e) {
    echo "foreach_ref: ", $e->getMessage(), "\n";
}
unset($r);
echo "refs: ", $refs[0], " ", $refs[1], " ", $refs[2], "\n";

$bag = new Bag();
try {
    foreach ($bag as $name => $value) {
        if ($value == 2) {
            throw new Exception("bag $name");
        }
        echo "bag: ", $name, "=", $value, "\n";
    }
} catch (Exception $e) {
    echo "object_iter: ", $e->getMessage(), "\n";
}
echo "end\n";

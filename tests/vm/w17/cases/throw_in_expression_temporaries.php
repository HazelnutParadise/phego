<?php
// throw_in_expression_temporaries: exceptions while temporaries are alive, with destructor order and unchanged targets
class Tracker {
    public $n;

    function __construct($n) {
        $this->n = $n;
    }

    function __destruct() {
        echo "destroy: ", $this->n, "\n";
    }
}

class Dict {
    public $p = "orig";
}

function thrower() {
    throw new Exception("temp");
}

function h($x) {
    echo "h called\n";
    return $x;
}

function g($x) {
    echo "g called\n";
    return $x;
}

echo "case1 start\n";
try {
    $s = "a" . (new Tracker("t1"))->n . thrower();
} catch (Exception $e) {
    echo "case1 caught\n";
}
echo "s_set: ", (isset($s) ? 1 : 0), "\n";

echo "case2 start\n";
try {
    $arr = [new Tracker("p"), thrower(), new Tracker("q")];
} catch (Exception $e) {
    echo "case2 caught\n";
}
echo "arr_set: ", (isset($arr) ? 1 : 0), "\n";

echo "case3 start\n";
$list = [];
try {
    $list[] = thrower();
} catch (Exception $e) {
    echo "case3 caught\n";
}
echo "list_has_zero: ", (isset($list[0]) ? 1 : 0), "\n";

echo "case4 start\n";
$o = new Dict();
try {
    $o->p = thrower();
} catch (Exception $e) {
    echo "case4 caught\n";
}
echo "p_value: ", $o->p, "\n";

echo "case5 start\n";
try {
    $nested = g(h(thrower()));
} catch (Exception $e) {
    echo "case5 caught\n";
}
echo "nested_set: ", (isset($nested) ? 1 : 0), "\n";

echo "case6 start\n";
$keep = new Tracker("keep");
try {
    $keep = [new Tracker("inside"), thrower()];
} catch (Exception $e) {
    echo "case6 caught\n";
}
echo "keep_still_object: ", ($keep instanceof Tracker ? 1 : 0), "\n";
echo "end\n";

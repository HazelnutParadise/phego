<?php
// throw_in_argument_evaluation: an exception while evaluating call arguments skips the call and destroys evaluated temporaries
class Tracker {
    public $n;

    function __construct($n) {
        $this->n = $n;
        echo "make ", $n, "\n";
    }

    function __destruct() {
        echo "destroy: ", $this->n, "\n";
    }
}

class Obj {
    function __construct($a, $b) {
        echo "ctor called\n";
    }

    function m($a, $b) {
        echo "m called\n";
        return 2;
    }
}

function thrower() {
    echo "thrower runs\n";
    throw new Exception("arg failed");
}

function f($a, $b, $c) {
    echo "f called\n";
    return 1;
}

try {
    f(new Tracker("a"), thrower(), new Tracker("b"));
} catch (Exception $e) {
    echo "caught f: ", $e->getMessage(), "\n";
}

$obj = new Obj(1, 2);
try {
    $obj->m(new Tracker("c"), thrower());
} catch (Exception $e2) {
    echo "caught method: ", $e2->getMessage(), "\n";
}

$made = null;
try {
    $made = new Obj(new Tracker("d"), thrower());
} catch (Exception $e3) {
    echo "caught new: ", $e3->getMessage(), "\n";
}
echo "made_is_null: ", ($made === null ? 1 : 0), "\n";
echo "end\n";

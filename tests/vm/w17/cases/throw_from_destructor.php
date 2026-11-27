<?php
// throw_from_destructor: destructors that throw during unset, overwrite, null assignment and function return
class Noisy {
    public $n;

    function __construct($n) {
        $this->n = $n;
    }

    function __destruct() {
        echo "destructor ", $this->n, "\n";
        throw new Exception("from destructor " . $this->n);
    }
}

$a = new Noisy("a");
try {
    unset($a);
    echo "not reached 1\n";
} catch (Exception $e) {
    echo "caught1: ", $e->getMessage(), "\n";
}
echo "a_is_gone: ", (isset($a) ? 0 : 1), "\n";

$b = new Noisy("b");
try {
    $b = null;
    echo "not reached 2\n";
} catch (Exception $e) {
    echo "caught2: ", $e->getMessage(), "\n";
}
echo "b_is_null: ", ($b === null ? 1 : 0), "\n";

function make_and_drop() {
    $c = new Noisy("c");
    return 5;
}
try {
    $r = make_and_drop();
    echo "not reached 3\n";
} catch (Exception $e) {
    echo "caught3: ", $e->getMessage(), "\n";
}
echo "r_was_set: ", (isset($r) ? 1 : 0), "\n";

$d = new Noisy("d0");
try {
    $d = new Noisy("d1");
    echo "not reached 4\n";
} catch (Exception $e) {
    echo "caught4: ", $e->getMessage(), "\n";
}
echo "d_holds: ", $d->n, "\n";
try {
    $d = null;
} catch (Exception $e) {
    echo "caught5: ", $e->getMessage(), "\n";
}
echo "end\n";

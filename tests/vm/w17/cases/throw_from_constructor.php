<?php
// throw_from_constructor: a throwing constructor skips the destructor, keeps properties set before the throw and leaves the variable unchanged
class C {
    public $a;

    function __construct($fail) {
        global $seen;
        $seen = $this;
        $this->a = "set";
        if ($fail) {
            throw new Exception("ctor");
        }
        $this->a = "done";
    }

    function __destruct() {
        echo "C destruct\n";
    }
}

$seen = null;
$v = "old";
try {
    $v = new C(true);
    echo "not reached\n";
} catch (Exception $e) {
    echo "caught: ", $e->getMessage(), "\n";
}
echo "v_still_old: ", ($v === "old" ? 1 : 0), "\n";
echo "seen_is_object: ", ($seen instanceof C ? 1 : 0), "\n";
echo "property_before_throw: ", $seen->a, "\n";

$seen = null;
echo "after seen released\n";

try {
    $ok = new C(false);
    echo "ok property: ", $ok->a, "\n";
} catch (Exception $e2) {
    echo "not reached ok\n";
}
$ok = null;
echo "ok_still_held_by_seen: ", ($seen === null ? 0 : 1), "\n";
$seen = null;
echo "after ok released\n";
echo "end\n";

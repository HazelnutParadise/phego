<?php
// exception_property_access: protected message and code from outside, inside a subclass, dynamic property and isset
class Peek extends Exception {
    public function peek() {
        return "inside=" . $this->message . "/" . $this->code;
    }

    public function setInside($m) {
        $this->message = $m;
    }
}

$p = new Peek("hello", 4);
echo "peek: ", $p->peek(), "\n";
$p->setInside("changed");
echo "after_set: ", $p->getMessage(), "\n";

try {
    $outside = $p->message;
    echo "not reached read\n";
} catch (Error $err) {
    echo "outside_read_class: ", $err::class, "\n";
    echo "outside_read_message: ", $err->getMessage(), "\n";
}

echo "isset_protected: ", (isset($p->message) ? 1 : 0), "\n";

$p->extra = 1;
echo "extra_read: ", $p->extra, "\n";
echo "isset_extra: ", (isset($p->extra) ? 1 : 0), "\n";
unset($p->extra);
echo "isset_extra_after_unset: ", (isset($p->extra) ? 1 : 0), "\n";

try {
    $p->code = 9;
    echo "not reached write\n";
} catch (Error $err2) {
    echo "write_protected_class: ", $err2::class, "\n";
    echo "write_protected_message: ", $err2->getMessage(), "\n";
}
echo "code_still: ", $p->getCode(), "\n";
echo "end\n";

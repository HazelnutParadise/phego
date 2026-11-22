<?php
// try_catch_method_chain: an exception in the middle of a method chain; what ran before it; nullsafe chains that short-circuit
class Chain
{
    public $steps = [];

    public function a()
    {
        $this->steps[] = "a";
        echo "a ran\n";
        return $this;
    }

    public function b($fail = false)
    {
        $this->steps[] = "b";
        echo "b ran\n";
        if ($fail) {
            throw new RuntimeException("b failed");
        }
        return $this;
    }

    public function c()
    {
        $this->steps[] = "c";
        echo "c ran\n";
        return $this;
    }

    public function nothing()
    {
        return null;
    }
}

function argFail()
{
    throw new RuntimeException("argument failed");
}

$o = new Chain();
try {
    $o->a()->b(true)->c();
    echo "not reached\n";
} catch (RuntimeException $e) {
    echo "chain caught: ", $e->getMessage(), "\n";
}
foreach ($o->steps as $s) {
    echo "step recorded: ", $s, "\n";
}

$o2 = new Chain();
try {
    $o2->a()->b(argFail())->c();
} catch (RuntimeException $e) {
    echo "argument caught: ", $e->getMessage(), "\n";
}
foreach ($o2->steps as $s) {
    echo "o2 step recorded: ", $s, "\n";
}

$none = null;
$res = $none?->a()->b(true);
echo "nullsafe short-circuit null: ", $res === null ? "yes" : "no", "\n";

$o3 = new Chain();
try {
    $o3->nothing()->a();
} catch (Error $e) {
    echo "error: ", $e->getMessage(), "\n";
}
$res = $o3->nothing()?->a();
echo "nullsafe after null: ", $res === null ? "yes" : "no", "\n";

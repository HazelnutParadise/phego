<?php
// The nullsafe operator cannot be combined with first-class callable syntax.
// The whole file fails to compile, so none of the code below runs.

class A
{
    public function m($x)
    {
        return $x;
    }
}

$o = new A();
$direct = $o->m(...);
echo "direct callable: ", $direct(1), "\n";

$plain = $o->m(2);
echo "plain call: ", $plain, "\n";

$n = null;
echo "nullsafe on null: ", $n?->m(3) ?? "none", "\n";

// Invalid: nullsafe combined with (...).
$bad = $o?->m(...);

echo "not reached\n";
